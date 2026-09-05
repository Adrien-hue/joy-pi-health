package app

import (
	"context"
	"errors"
	"time"
)

type observerRuntime interface {
	Close(context.Context) error
}

// productionRuntime owns the HTTP server and the only periodic observer as one
// foreground lifecycle unit.
type productionRuntime struct {
	server   httpRuntime
	observer observerRuntime
	firmware observerRuntime
}

func (runtime *productionRuntime) Serve() error {
	return runtime.server.Serve()
}

func (runtime *productionRuntime) Shutdown(ctx context.Context) error {
	serverErr := runtime.server.Shutdown(ctx)
	observerErr := runtime.observer.Close(ctx)
	var firmwareErr error
	if runtime.firmware != nil {
		firmwareErr = runtime.firmware.Close(ctx)
	}
	return errors.Join(serverErr, observerErr, firmwareErr)
}

func (runtime *productionRuntime) Close() error {
	serverErr := runtime.server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	observerErr := runtime.observer.Close(ctx)
	var firmwareErr error
	if runtime.firmware != nil {
		firmwareErr = runtime.firmware.Close(ctx)
	}
	return errors.Join(serverErr, observerErr, firmwareErr)
}
