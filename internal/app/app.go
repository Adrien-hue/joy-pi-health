package app

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
	"github.com/Adrien-hue/joy-pi-health/internal/httpapi"
	"github.com/Adrien-hue/joy-pi-health/internal/observe"
	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

const shutdownTimeout = 2 * time.Second

type httpRuntime interface {
	Serve() error
	Shutdown(context.Context) error
	Close() error
}

type lifecycleDependencies struct {
	signalContext func() (context.Context, context.CancelFunc)
	startHTTP     func(context.Context, config.Config, func(error), *lineLogger) (httpRuntime, error)
	notifyReady   func([]string) error
}

// Run is the process-level application boundary.
//
// Run resolves configuration and owns the foreground runtime lifecycle.
func Run(args []string, environment []string, stdout, stderr io.Writer) int {
	return run(args, environment, stdout, stderr, lifecycleDependencies{
		signalContext: newSignalContext,
		startHTTP:     startHTTP,
		notifyReady:   notifySystemdReady,
	})
}

func run(args []string, environment []string, stdout, stderr io.Writer, dependencies lifecycleDependencies) int {
	resolved, err := config.Resolve(args, environment)
	if errors.Is(err, config.ErrHelp) {
		_, _ = io.WriteString(stdout, config.HelpText)
		return 0
	}
	if err != nil {
		_, _ = io.WriteString(stderr, "joy-pi-health: configuration error: "+err.Error()+"\n")
		return 2
	}
	logger := newLineLogger(stderr, resolved.LogLevel())
	if dependencies.notifyReady == nil {
		dependencies.notifyReady = func([]string) error { return nil }
	}

	ctx, stopSignals := dependencies.signalContext()
	defer stopSignals()
	if ctx.Err() != nil {
		return 0
	}

	fatalResult := make(chan error, 1)
	var fatalOnce sync.Once
	readiness := &readinessGate{}
	notifyFatal := func(fatalErr error) {
		fatalOnce.Do(func() {
			readiness.markFatal()
			fatalResult <- fatalErr
		})
	}
	server, err := dependencies.startHTTP(ctx, resolved, notifyFatal, logger)
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		if readiness.hasFatal() {
			logger.error("operational error: internal service failure")
		} else {
			logger.error("operational error: service startup failed")
		}
		return 1
	}

	serveResult := make(chan error, 1)
	serveStarted := make(chan struct{})
	go func() {
		close(serveStarted)
		serveResult <- server.Serve()
	}()
	<-serveStarted

	select {
	case <-ctx.Done():
		return shutDown(server, serveResult, logger)
	case <-fatalResult:
		logger.error("operational error: internal service failure")
		_ = shutDown(server, serveResult, logger)
		return 1
	case serveErr := <-serveResult:
		return unexpectedServeExit(server, serveErr, logger)
	default:
	}
	if err := readiness.notify(ctx, func() error { return dependencies.notifyReady(environment) }); err != nil {
		if ctx.Err() != nil {
			return shutDown(server, serveResult, logger)
		}
		if readiness.hasFatal() {
			logger.error("operational error: internal service failure")
		} else {
			logger.error("operational error: readiness notification failed")
		}
		_ = shutDown(server, serveResult, logger)
		return 1
	}

	select {
	case <-ctx.Done():
		return shutDown(server, serveResult, logger)
	case <-fatalResult:
		logger.error("operational error: internal service failure")
		_ = shutDown(server, serveResult, logger)
		return 1
	case serveErr := <-serveResult:
		return unexpectedServeExit(server, serveErr, logger)
	}
}

func startHTTP(ctx context.Context, resolved config.Config, fatal func(error), logger *lineLogger) (httpRuntime, error) {
	source := platform.NewSource()
	observer, err := observe.NewCPUObserver(source, fatal)
	if err != nil {
		return nil, err
	}
	firmware, err := observe.NewFirmwareExecutor(platform.NewFirmwareTransaction(), fatal)
	if err != nil {
		_ = observer.Close(context.Background())
		return nil, err
	}
	if err := firmware.Start(); err != nil {
		_ = observer.Close(context.Background())
		return nil, err
	}
	reporter, err := observe.NewDegradationReporter(logger.degradation)
	if err != nil {
		_ = observer.Close(context.Background())
		_ = firmware.Close(context.Background())
		return nil, err
	}
	provider, err := observe.NewCoordinatorWithRaspberryPi(ctx, source, observer, platform.NewThermalSource(), firmware, reporter, fatal)
	if err != nil {
		_ = observer.Close(context.Background())
		_ = firmware.Close(context.Background())
		return nil, err
	}
	server, err := httpapi.Listen(ctx, resolved.ListenAddress(), resolved.ListenPort(), provider, fatal, logger.errorWriter())
	if err != nil {
		_ = observer.Close(context.Background())
		_ = firmware.Close(context.Background())
		return nil, err
	}
	if err := observer.Start(); err != nil {
		_ = server.Close()
		closeContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = observer.Close(closeContext)
		_ = firmware.Close(closeContext)
		return nil, err
	}
	if err := observe.ProbeReadiness(ctx, source); err != nil {
		_ = server.Close()
		closeContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = observer.Close(closeContext)
		_ = firmware.Close(closeContext)
		return nil, err
	}
	if err := firmware.Ready(); err != nil {
		_ = server.Close()
		closeContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = observer.Close(closeContext)
		_ = firmware.Close(closeContext)
		return nil, err
	}
	return &productionRuntime{server: server, observer: observer, firmware: firmware}, nil
}

func unexpectedServeExit(server httpRuntime, serveErr error, logger *lineLogger) int {
	_ = server.Close()
	if serveErr != nil {
		logger.error("operational error: HTTP server failed")
	} else {
		logger.error("operational error: HTTP server stopped unexpectedly")
	}
	return 1
}

func shutDown(server httpRuntime, serveResult <-chan error, logger *lineLogger) int {
	deadline := time.Now().Add(shutdownTimeout)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	shutdownErr := server.Shutdown(ctx)
	if shutdownErr != nil {
		_ = server.Close()
	}

	remaining := time.Until(deadline)
	if remaining < 0 {
		remaining = 0
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()

	var serveErr error
	select {
	case serveErr = <-serveResult:
	case <-timer.C:
		_ = server.Close()
		logger.error("operational error: shutdown exceeded two seconds")
		return 1
	}

	if shutdownErr != nil {
		logger.error("operational error: graceful shutdown failed")
		return 1
	}
	if serveErr != nil {
		logger.error("operational error: HTTP server failed during shutdown")
		return 1
	}
	return 0
}
