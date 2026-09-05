package app

import (
	"context"
	"errors"
)

type observerRuntime interface {
	Close(context.Context) error
}

// productionRuntime owns the HTTP server and the only periodic observer as one
// foreground lifecycle unit.
type productionRuntime struct {
	server   httpRuntime
	observer observerRuntime
}

func (runtime *productionRuntime) Serve() error {
	return runtime.server.Serve()
}

func (runtime *productionRuntime) Shutdown(ctx context.Context) error {
	serverErr := runtime.server.Shutdown(ctx)
	observerErr := runtime.observer.Close(ctx)
	return errors.Join(serverErr, observerErr)
}

func (runtime *productionRuntime) Close() error {
	serverErr := runtime.server.Close()
	observerErr := runtime.observer.Close(context.Background())
	return errors.Join(serverErr, observerErr)
}
