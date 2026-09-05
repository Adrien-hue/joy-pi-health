package app

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
	"github.com/Adrien-hue/joy-pi-health/internal/httpapi"
)

const shutdownTimeout = 2 * time.Second

type httpRuntime interface {
	Serve() error
	Shutdown(context.Context) error
	Close() error
}

type lifecycleDependencies struct {
	signalContext func() (context.Context, context.CancelFunc)
	startHTTP     func(context.Context, config.Config, io.Writer) (httpRuntime, error)
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

	server, err := dependencies.startHTTP(ctx, resolved, stderr)
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

func startHTTP(ctx context.Context, resolved config.Config, errorOutput io.Writer) (httpRuntime, error) {
	return httpapi.Listen(ctx, resolved.ListenAddress(), resolved.ListenPort(), errorOutput)
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
