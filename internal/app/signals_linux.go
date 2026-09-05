//go:build linux

package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func newSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}
