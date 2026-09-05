package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
)

func TestRunConfigurationOutcomesPrecedeRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		environment []string
		wantStatus  int
		wantStdout  string
		wantError   bool
	}{
		{name: "help", args: []string{"--help"}, wantStatus: 0, wantStdout: config.HelpText},
		{
			name:        "help bypasses environment validation",
			args:        []string{"--help"},
			environment: []string{"JOY_PI_HEALTH_UNKNOWN=value"},
			wantStatus:  0,
			wantStdout:  config.HelpText,
		},
		{name: "command-line error", args: []string{"--listen-port=0"}, wantStatus: 2, wantError: true},
		{name: "environment error", environment: []string{"JOY_PI_HEALTH_UNKNOWN=value"}, wantStatus: 2, wantError: true},
		{name: "non-loopback acknowledgement error", args: []string{"--listen-address=192.168.1.2"}, wantStatus: 2, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			runtimeCalled := false
			dependencies := lifecycleDependencies{
				signalContext: func() (context.Context, context.CancelFunc) {
					runtimeCalled = true
					return context.WithCancel(context.Background())
				},
				startHTTP: func(context.Context, config.Config, func(error), io.Writer) (httpRuntime, error) {
					runtimeCalled = true
					return nil, errors.New("must not be called")
				},
			}

			status := run(test.args, test.environment, &stdout, &stderr, dependencies)
			if status != test.wantStatus {
				t.Errorf("run() status = %d, want %d", status, test.wantStatus)
			}
			if runtimeCalled {
				t.Error("runtime setup occurred before configuration outcome")
			}
			if stdout.String() != test.wantStdout {
				t.Errorf("run() stdout = %q, want %q", stdout.String(), test.wantStdout)
			}
			assertDiagnostic(t, stderr.String(), test.wantError, "configuration error")
		})
	}
}

func TestRunAlreadyCancelledDoesNotBind(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	startCalled := false
	dependencies := lifecycleDependencies{
		signalContext: func() (context.Context, context.CancelFunc) {
			return ctx, func() {}
		},
		startHTTP: func(context.Context, config.Config, func(error), io.Writer) (httpRuntime, error) {
			startCalled = true
			return nil, errors.New("must not be called")
		},
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if status := run(nil, nil, &stdout, &stderr, dependencies); status != 0 {
		t.Errorf("run() status = %d, want 0", status)
	}
	if startCalled {
		t.Error("HTTP runtime started after cancellation")
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("run() output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
	}
}

func TestRunServesUntilCleanCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	server := newFakeHTTPRuntime()
	status := make(chan int, 1)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	environment := []string{"NOTIFY_SOCKET=/must/not/be/contacted"}

	go func() {
		status <- run(nil, environment, &stdout, &stderr, dependenciesFor(ctx, server, nil))
	}()
	waitForSignal(t, server.started, "HTTP serve start")
	cancel()

	if got := waitForResult(t, status, "application shutdown"); got != 0 {
		t.Errorf("run() status = %d, want 0", got)
	}
	shutdownCalls, closeCalls := server.calls()
	if shutdownCalls != 1 || closeCalls != 0 {
		t.Errorf("runtime calls = shutdown %d, close %d; want 1, 0", shutdownCalls, closeCalls)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("run() output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
	}
}

func TestRunOccupiedPortIsOperationalFailure(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	status := run(
		[]string{"--listen-port=" + strconv.Itoa(port)},
		nil,
		&stdout,
		&stderr,
		lifecycleDependencies{signalContext: backgroundSignalContext, startHTTP: startHTTP},
	)
	if status != 1 {
		t.Errorf("run() status = %d, want 1", status)
	}
	if stdout.Len() != 0 {
		t.Errorf("run() stdout = %q, want none", stdout.String())
	}
	assertDiagnostic(t, stderr.String(), true, "operational error")
}

func TestRunUnexpectedServeTerminationIsOperationalFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newFakeHTTPRuntime()
	status := make(chan int, 1)
	var stderr bytes.Buffer
	go func() {
		status <- run(nil, nil, io.Discard, &stderr, dependenciesFor(ctx, server, nil))
	}()
	waitForSignal(t, server.started, "HTTP serve start")
	server.finish(errors.New("serve failed"))

	if got := waitForResult(t, status, "serve failure"); got != 1 {
		t.Errorf("run() status = %d, want 1", got)
	}
	_, closeCalls := server.calls()
	if closeCalls != 1 {
		t.Errorf("Close() calls = %d, want 1", closeCalls)
	}
	assertDiagnostic(t, stderr.String(), true, "operational error")
}

func TestRunShutdownFailureIsOperationalFailure(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	server := newFakeHTTPRuntime()
	server.shutdownErr = errors.New("shutdown failed")
	status := make(chan int, 1)
	var stderr bytes.Buffer
	go func() {
		status <- run(nil, nil, io.Discard, &stderr, dependenciesFor(ctx, server, nil))
	}()
	waitForSignal(t, server.started, "HTTP serve start")
	cancel()

	if got := waitForResult(t, status, "failed shutdown"); got != 1 {
		t.Errorf("run() status = %d, want 1", got)
	}
	shutdownCalls, closeCalls := server.calls()
	if shutdownCalls != 1 || closeCalls != 1 {
		t.Errorf("runtime calls = shutdown %d, close %d; want 1, 1", shutdownCalls, closeCalls)
	}
	assertDiagnostic(t, stderr.String(), true, "operational error")
}

func TestRunRepeatedStartStopJoinsServeGoroutine(t *testing.T) {
	for iteration := range 25 {
		ctx, cancel := context.WithCancel(context.Background())
		server := newFakeHTTPRuntime()
		status := make(chan int, 1)
		go func() {
			status <- run(nil, nil, io.Discard, io.Discard, dependenciesFor(ctx, server, nil))
		}()
		waitForSignal(t, server.started, "HTTP serve start")
		cancel()
		if got := waitForResult(t, status, "application shutdown"); got != 0 {
			t.Fatalf("iteration %d: run() status = %d, want 0", iteration, got)
		}
		waitForSignal(t, server.exited, "HTTP serve exit")
	}
}

func TestRunInternalHTTPFailureShutsDownAndExitsOne(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := newFakeHTTPRuntime()
	status := make(chan int, 1)
	var stderr bytes.Buffer
	fatalReady := make(chan func(error), 1)
	dependencies := lifecycleDependencies{
		signalContext: func() (context.Context, context.CancelFunc) { return ctx, func() {} },
		startHTTP: func(_ context.Context, _ config.Config, fatal func(error), _ io.Writer) (httpRuntime, error) {
			fatalReady <- fatal
			return server, nil
		},
	}
	go func() { status <- run(nil, nil, io.Discard, &stderr, dependencies) }()
	waitForSignal(t, server.started, "HTTP serve start")
	notifyFatal := waitForResult(t, fatalReady, "fatal notifier")
	notifyFatal(errors.New("internal defect"))

	if got := waitForResult(t, status, "fatal shutdown"); got != 1 {
		t.Errorf("run() status = %d, want 1", got)
	}
	shutdownCalls, _ := server.calls()
	if shutdownCalls != 1 {
		t.Errorf("Shutdown() calls = %d, want 1", shutdownCalls)
	}
	assertDiagnostic(t, stderr.String(), true, "operational error")
}

func TestProductionRuntimeShutsDownServerBeforeObserver(t *testing.T) {
	t.Parallel()
	server := newFakeHTTPRuntime()
	order := make(chan string, 2)
	server.shutdownHook = func() { order <- "server" }
	observer := &fakeObserverRuntime{close: func(context.Context) error {
		order <- "observer"
		return nil
	}}
	runtime := &productionRuntime{server: server, observer: observer}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if first, second := <-order, <-order; first != "server" || second != "observer" {
		t.Fatalf("shutdown order = %q, %q", first, second)
	}
}

func TestProductionRuntimeShutsDownFirmwareAfterCPUObserver(t *testing.T) {
	t.Parallel()
	server := newFakeHTTPRuntime()
	order := make(chan string, 3)
	server.shutdownHook = func() { order <- "server" }
	observer := &fakeObserverRuntime{close: func(context.Context) error { order <- "observer"; return nil }}
	firmware := &fakeObserverRuntime{close: func(context.Context) error { order <- "firmware"; return nil }}
	runtime := &productionRuntime{server: server, observer: observer, firmware: firmware}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if first, second, third := <-order, <-order, <-order; first != "server" || second != "observer" || third != "firmware" {
		t.Fatalf("shutdown order = %q, %q, %q", first, second, third)
	}
}

type fakeObserverRuntime struct {
	close func(context.Context) error
}

func (observer *fakeObserverRuntime) Close(ctx context.Context) error {
	return observer.close(ctx)
}

func dependenciesFor(ctx context.Context, server httpRuntime, startErr error) lifecycleDependencies {
	return lifecycleDependencies{
		signalContext: func() (context.Context, context.CancelFunc) {
			return ctx, func() {}
		},
		startHTTP: func(context.Context, config.Config, func(error), io.Writer) (httpRuntime, error) {
			return server, startErr
		},
	}
}

func backgroundSignalContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func assertDiagnostic(t *testing.T, diagnostic string, want bool, category string) {
	t.Helper()
	if !want {
		if diagnostic != "" {
			t.Errorf("stderr = %q, want none", diagnostic)
		}
		return
	}
	if !strings.HasPrefix(diagnostic, "joy-pi-health: "+category+": ") {
		t.Errorf("stderr = %q, want %s prefix", diagnostic, category)
	}
	if !strings.HasSuffix(diagnostic, "\n") {
		t.Errorf("stderr = %q, want trailing newline", diagnostic)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForResult[T any](t *testing.T, results <-chan T, description string) T {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}

type fakeHTTPRuntime struct {
	started chan struct{}
	exited  chan struct{}
	result  chan error
	once    sync.Once

	mu            sync.Mutex
	shutdownCalls int
	closeCalls    int
	shutdownErr   error
	shutdownHook  func()
}

func newFakeHTTPRuntime() *fakeHTTPRuntime {
	return &fakeHTTPRuntime{
		started: make(chan struct{}),
		exited:  make(chan struct{}),
		result:  make(chan error, 1),
	}
}

func (s *fakeHTTPRuntime) Serve() error {
	close(s.started)
	err := <-s.result
	close(s.exited)
	return err
}

func (s *fakeHTTPRuntime) Shutdown(context.Context) error {
	s.mu.Lock()
	s.shutdownCalls++
	err := s.shutdownErr
	hook := s.shutdownHook
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err == nil {
		s.finish(nil)
	}
	return err
}

func (s *fakeHTTPRuntime) Close() error {
	s.mu.Lock()
	s.closeCalls++
	s.mu.Unlock()
	s.finish(nil)
	return nil
}

func (s *fakeHTTPRuntime) finish(err error) {
	s.once.Do(func() { s.result <- err })
}

func (s *fakeHTTPRuntime) calls() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutdownCalls, s.closeCalls
}
