package observe

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

type firmwareObservationProvider interface {
	Observe(context.Context, *cycleToken) outcome[firmwareHealth]
}

type firmwareRequest struct {
	token  *cycleToken
	result chan firmwareResult
}

type firmwareResult struct {
	token   *cycleToken
	outcome outcome[firmwareHealth]
}

// FirmwareExecutor owns the sole potentially blocking firmware transaction.
// Admission reserves the sole slot before a request is handed to the worker,
// deliberately providing single-flight, no-queue behavior.
type FirmwareExecutor struct {
	transaction platform.FirmwareTransaction
	fatal       func(error)
	requests    chan firmwareRequest
	stop        chan struct{}
	done        chan struct{}

	mu       sync.Mutex
	started  bool
	stopping bool
	busy     bool
	failed   error
	stopOnce sync.Once
}

func NewFirmwareExecutor(transaction platform.FirmwareTransaction, fatal func(error)) (*FirmwareExecutor, error) {
	if transaction == nil {
		return nil, errors.New("firmware transaction is required")
	}
	if fatal == nil {
		return nil, errors.New("fatal notifier is required")
	}
	return &FirmwareExecutor{
		transaction: transaction,
		fatal:       fatal,
		requests:    make(chan firmwareRequest, 1),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
	}, nil
}

func (executor *FirmwareExecutor) Start() error {
	executor.mu.Lock()
	defer executor.mu.Unlock()
	if executor.started || executor.stopping || executor.failed != nil {
		return errors.New("firmware executor cannot be started")
	}
	executor.started = true
	go executor.run()
	return nil
}

func (executor *FirmwareExecutor) Observe(ctx context.Context, token *cycleToken) outcome[firmwareHealth] {
	if ctx == nil || token == nil {
		return defect[firmwareHealth](errors.New("firmware observation requires a context and cycle identity"))
	}
	executor.mu.Lock()
	started, stopping, busy, failed := executor.started, executor.stopping, executor.busy, executor.failed
	if failed != nil {
		executor.mu.Unlock()
		return defect[firmwareHealth](failed)
	}
	if !started || stopping {
		executor.mu.Unlock()
		return defect[firmwareHealth](errors.New("firmware executor is not running"))
	}
	if busy {
		executor.mu.Unlock()
		return unavailable[firmwareHealth](reasonTemporarilyUnavailable, errors.New("firmware executor is busy"))
	}
	executor.busy = true
	executor.mu.Unlock()
	request := firmwareRequest{token: token, result: make(chan firmwareResult, 1)}
	executor.requests <- request
	select {
	case result := <-request.result:
		if result.token != token {
			return defect[firmwareHealth](errors.New("firmware result has the wrong cycle identity"))
		}
		return result.outcome
	case <-ctx.Done():
		return unavailable[firmwareHealth](reasonTemporarilyUnavailable, ctx.Err())
	}
}

func (executor *FirmwareExecutor) run() {
	var active *firmwareRequest
	defer func() {
		if recovered := recover(); recovered != nil {
			err := internalDefect(fmt.Errorf("firmware executor panic: %v", recovered))
			executor.mu.Lock()
			executor.failed = err
			executor.mu.Unlock()
			if active != nil {
				active.result <- firmwareResult{token: active.token, outcome: defect[firmwareHealth](err)}
			}
			executor.fatal(err)
		}
		close(executor.done)
	}()
	for {
		select {
		case <-executor.stop:
			return
		case request := <-executor.requests:
			if request.token == nil {
				panic("firmware executor accepted an empty cycle identity")
			}
			active = &request
			mask, err := executor.transaction.GetThrottled()
			result := present(firmwareHealthFromMask(mask))
			if err != nil {
				result = acquisitionFailure[firmwareHealth](err, reasonUnsupported)
			}
			select {
			case request.result <- firmwareResult{token: request.token, outcome: result}:
			default:
			}
			executor.mu.Lock()
			executor.busy = false
			executor.mu.Unlock()
			active = nil
		}
	}
}

func (executor *FirmwareExecutor) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("firmware executor close context is nil")
	}
	executor.mu.Lock()
	if !executor.started {
		executor.mu.Unlock()
		return nil
	}
	executor.stopping = true
	executor.mu.Unlock()
	executor.stopOnce.Do(func() { close(executor.stop) })
	select {
	case <-executor.done:
		return nil
	case <-ctx.Done():
		// A kernel ioctl cannot be canceled safely. The process-level shutdown
		// bound is authoritative and process exit terminates the read-only call.
		return nil
	}
}
