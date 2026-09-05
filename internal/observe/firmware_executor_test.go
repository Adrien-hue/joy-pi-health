package observe

import (
	"context"
	"errors"
	"io/fs"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

type firmwareTransactionStub struct {
	call func() (uint32, error)
}

func (transaction firmwareTransactionStub) GetThrottled() (uint32, error) { return transaction.call() }

func TestFirmwareExecutorSuccessAndFailureClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		value  uint32
		err    error
		state  outcomeState
		reason availabilityReason
	}{
		{"success", 1 << 2, nil, statePresent, 0},
		{"unsupported", 0, fs.ErrNotExist, stateUnavailable, reasonUnsupported},
		{"permission", 0, fs.ErrPermission, stateUnavailable, reasonPermissionDenied},
		{"temporary", 0, errors.New("firmware failure"), stateUnavailable, reasonTemporarilyUnavailable},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			executor := mustFirmwareExecutor(t, firmwareTransactionStub{call: func() (uint32, error) { return test.value, test.err }}, func(error) {})
			result := executor.Observe(context.Background(), &cycleToken{identity: 1})
			if result.state != test.state || result.reason != test.reason {
				t.Fatalf("result = %#v", result)
			}
			if test.err == nil && !result.value.thermalThrottlingActive {
				t.Fatalf("flags = %#v", result.value)
			}
			_ = executor.Close(context.Background())
		})
	}
}

func TestFirmwareExecutorBusyTimeoutLateDiscardAndRecovery(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	executor := mustFirmwareExecutor(t, firmwareTransactionStub{call: func() (uint32, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return 0, nil
	}}, func(error) {})

	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan outcome[firmwareHealth], 1)
	go func() { first <- executor.Observe(ctx, &cycleToken{identity: 1}) }()
	<-entered
	busy := executor.Observe(context.Background(), &cycleToken{identity: 2})
	if busy.state != stateUnavailable || busy.reason != reasonTemporarilyUnavailable || calls.Load() != 1 {
		t.Fatalf("busy result = %#v, calls = %d", busy, calls.Load())
	}
	cancel()
	if result := <-first; result.state != stateUnavailable {
		t.Fatalf("timed out result = %#v", result)
	}
	close(release)
	for attempt := 0; attempt < 10000; attempt++ {
		result := executor.Observe(context.Background(), &cycleToken{identity: 3})
		if result.state == statePresent {
			if calls.Load() != 2 {
				t.Fatalf("calls = %d", calls.Load())
			}
			_ = executor.Close(context.Background())
			return
		}
		runtime.Gosched()
	}
	t.Fatal("executor did not recover after the late result")
}

func TestFirmwareExecutorPanicIsFatal(t *testing.T) {
	t.Parallel()
	fatal := make(chan error, 1)
	executor := mustFirmwareExecutor(t, firmwareTransactionStub{call: func() (uint32, error) { panic("broken ioctl") }}, func(err error) { fatal <- err })
	result := executor.Observe(context.Background(), &cycleToken{identity: 1})
	if result.state != stateDefect || !isInternalDefect(result.cause) {
		t.Fatalf("result = %#v", result)
	}
	select {
	case err := <-fatal:
		if !isInternalDefect(err) {
			t.Fatalf("fatal = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("fatal notification was not delivered")
	}
	_ = executor.Close(context.Background())
}

func TestFirmwareExecutorShutdownDoesNotWaitForBlockedTransaction(t *testing.T) {
	t.Parallel()
	entered := make(chan struct{})
	release := make(chan struct{})
	executor := mustFirmwareExecutor(t, firmwareTransactionStub{call: func() (uint32, error) {
		close(entered)
		<-release
		return 0, nil
	}}, func(error) {})
	ctx, cancel := context.WithCancel(context.Background())
	go executor.Observe(ctx, &cycleToken{identity: 1})
	<-entered
	cancel()
	closeCtx, closeCancel := context.WithCancel(context.Background())
	closeCancel()
	if err := executor.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case <-executor.done:
	case <-time.After(time.Second):
		t.Fatal("executor did not terminate after the blocked transaction completed")
	}
}

func mustFirmwareExecutor(t *testing.T, transaction firmwareTransactionStub, fatal func(error)) *FirmwareExecutor {
	t.Helper()
	executor, err := NewFirmwareExecutor(transaction, fatal)
	if err != nil {
		t.Fatal(err)
	}
	if err := executor.Start(); err != nil {
		t.Fatal(err)
	}
	return executor
}
