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
	startHTTP     func(context.Context, config.Config, func(error), io.Writer) (httpRuntime, error)
}

// Run is the process-level application boundary.
//
// Run resolves configuration and owns the foreground runtime lifecycle.
func Run(args []string, environment []string, stdout, stderr io.Writer) int {
	return run(args, environment, stdout, stderr, lifecycleDependencies{
		signalContext: newSignalContext,
		startHTTP:     startHTTP,
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

	ctx, stopSignals := dependencies.signalContext()
	defer stopSignals()
	if ctx.Err() != nil {
		return 0
	}

	fatalResult := make(chan error, 1)
	var fatalOnce sync.Once
	notifyFatal := func(fatalErr error) {
		fatalOnce.Do(func() { fatalResult <- fatalErr })
	}
	server, err := dependencies.startHTTP(ctx, resolved, notifyFatal, stderr)
	if err != nil {
		if ctx.Err() != nil {
			return 0
		}
		writeOperationalError(stderr, "could not bind the configured listener")
		return 1
	}

	serveResult := make(chan error, 1)
	go func() {
		serveResult <- server.Serve()
	}()

	// Systemd readiness remains intentionally dormant until the CPU baseline,
	// firmware executor, and final request admission gates all exist.
	select {
	case <-ctx.Done():
		return shutDown(server, serveResult, stderr)
	case <-fatalResult:
		writeOperationalError(stderr, "internal service failure")
		_ = shutDown(server, serveResult, stderr)
		return 1
	case serveErr := <-serveResult:
		_ = server.Close()
		if serveErr != nil {
			writeOperationalError(stderr, "HTTP server failed")
		} else {
			writeOperationalError(stderr, "HTTP server stopped unexpectedly")
		}
		return 1
	}
}

func startHTTP(ctx context.Context, resolved config.Config, fatal func(error), errorOutput io.Writer) (httpRuntime, error) {
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
	provider, err := observe.NewCoordinatorWithRaspberryPi(ctx, source, observer, platform.NewThermalSource(), firmware, fatal)
	if err != nil {
		_ = observer.Close(context.Background())
		_ = firmware.Close(context.Background())
		return nil, err
	}
	server, err := httpapi.Listen(ctx, resolved.ListenAddress(), resolved.ListenPort(), provider, fatal, errorOutput)
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
	return &productionRuntime{server: server, observer: observer, firmware: firmware}, nil
}

func shutDown(server httpRuntime, serveResult <-chan error, stderr io.Writer) int {
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
		writeOperationalError(stderr, "shutdown exceeded two seconds")
		return 1
	}

	if shutdownErr != nil {
		writeOperationalError(stderr, "graceful shutdown failed")
		return 1
	}
	if serveErr != nil {
		writeOperationalError(stderr, "HTTP server failed during shutdown")
		return 1
	}
	return 0
}

func writeOperationalError(stderr io.Writer, message string) {
	_, _ = io.WriteString(stderr, "joy-pi-health: operational error: "+message+"\n")
}
