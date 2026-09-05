package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestNotifySystemdReadyWith(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		environment []string
		wantSocket  string
		wantCalls   int
	}{
		{name: "missing socket"},
		{name: "empty socket", environment: []string{"NOTIFY_SOCKET="}},
		{name: "filesystem socket", environment: []string{"NOTIFY_SOCKET=/run/systemd/notify"}, wantSocket: "/run/systemd/notify", wantCalls: 1},
		{name: "abstract socket", environment: []string{"NOTIFY_SOCKET=@systemd-notify"}, wantSocket: "\x00systemd-notify", wantCalls: 1},
		{name: "last effective value", environment: []string{"NOTIFY_SOCKET=/first", "NOTIFY_SOCKET=/second"}, wantSocket: "/second", wantCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			err := notifySystemdReadyWith(test.environment, func(socket string, payload []byte, timeout time.Duration) error {
				calls++
				if socket != test.wantSocket {
					t.Errorf("socket = %q, want %q", socket, test.wantSocket)
				}
				if string(payload) != readyPayload {
					t.Errorf("payload = %q, want %q", payload, readyPayload)
				}
				if timeout != readinessWriteTimeout {
					t.Errorf("timeout = %v, want %v", timeout, readinessWriteTimeout)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("notifySystemdReadyWith() error = %v", err)
			}
			if calls != test.wantCalls {
				t.Errorf("sender calls = %d, want %d", calls, test.wantCalls)
			}
		})
	}
}

func TestReadinessGateNotifiesExactlyOnce(t *testing.T) {
	t.Parallel()
	gate := &readinessGate{}
	var calls atomic.Int32
	send := func() error { calls.Add(1); return nil }
	if err := gate.notify(context.Background(), send); err != nil {
		t.Fatal(err)
	}
	if err := gate.notify(context.Background(), send); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || !gate.ready {
		t.Fatalf("calls = %d, ready = %t", calls.Load(), gate.ready)
	}
}

func TestReadinessGateRejectsFatalAndCancellation(t *testing.T) {
	t.Parallel()
	gate := &readinessGate{}
	gate.markFatal()
	if err := gate.notify(context.Background(), func() error { t.Fatal("sender called"); return nil }); err == nil {
		t.Fatal("fatal readiness notification succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (&readinessGate{}).notify(ctx, func() error { t.Fatal("sender called"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled notification error = %v", err)
	}
}

func TestReadinessGateRetainsNotifierFailure(t *testing.T) {
	t.Parallel()
	want := errors.New("notify failed")
	gate := &readinessGate{}
	var calls atomic.Int32
	send := func() error { calls.Add(1); return want }
	if err := gate.notify(context.Background(), send); !errors.Is(err, want) {
		t.Fatalf("notification error = %v", err)
	}
	if err := gate.notify(context.Background(), func() error { t.Fatal("second sender called"); return nil }); !errors.Is(err, want) {
		t.Fatalf("repeated notification error = %v", err)
	}
	if calls.Load() != 1 || gate.ready {
		t.Fatalf("calls = %d, ready = %t", calls.Load(), gate.ready)
	}
}

func TestNotifySystemdReadyWithPropagatesError(t *testing.T) {
	t.Parallel()

	want := errors.New("send failed")
	err := notifySystemdReadyWith([]string{"NOTIFY_SOCKET=/run/systemd/notify"}, func(string, []byte, time.Duration) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("notifySystemdReadyWith() error = %v, want %v", err, want)
	}
}
