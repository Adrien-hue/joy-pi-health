package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	systemdNotificationSocket = "NOTIFY_SOCKET"
	readyPayload              = "READY=1"
	readinessWriteTimeout     = 100 * time.Millisecond
)

type readinessSender func(string, []byte, time.Duration) error

type readinessGate struct {
	mu        sync.Mutex
	attempted bool
	ready     bool
	fatal     bool
	result    error
	detected  atomic.Bool
}

func (gate *readinessGate) markFatal() {
	gate.detected.Store(true)
	gate.mu.Lock()
	gate.fatal = true
	gate.mu.Unlock()
}

func (gate *readinessGate) hasFatal() bool { return gate.detected.Load() }

func (gate *readinessGate) notify(ctx context.Context, send func() error) error {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	if gate.fatal || gate.detected.Load() {
		return errors.New("fatal lifecycle transition prevents readiness")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if gate.attempted {
		return gate.result
	}
	gate.attempted = true
	gate.result = send()
	if gate.detected.Load() {
		gate.result = errors.New("fatal lifecycle transition prevents readiness")
	}
	gate.ready = gate.result == nil
	return gate.result
}

func notifySystemdReady(environment []string) error {
	return notifySystemdReadyWith(environment, sendSystemdDatagram)
}

func notifySystemdReadyWith(environment []string, send readinessSender) error {
	socket := effectiveEnvironmentValue(environment, systemdNotificationSocket)
	if socket == "" {
		return nil
	}
	return send(systemdSocketAddress(socket), []byte(readyPayload), readinessWriteTimeout)
}

func effectiveEnvironmentValue(environment []string, key string) string {
	value := ""
	for _, entry := range environment {
		name, candidate, found := strings.Cut(entry, "=")
		if found && name == key {
			value = candidate
		}
	}
	return value
}

func systemdSocketAddress(socket string) string {
	if strings.HasPrefix(socket, "@") {
		return "\x00" + strings.TrimPrefix(socket, "@")
	}
	return socket
}
