//go:build !linux

package app

import (
	"context"
	"os"
	"os/signal"
)

func newSignalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
