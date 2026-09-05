package app

import (
	"strings"
	"time"
)

const (
	systemdNotificationSocket = "NOTIFY_SOCKET"
	readyPayload              = "READY=1"
	readinessWriteTimeout     = 100 * time.Millisecond
)

type readinessSender func(string, []byte, time.Duration) error

// notifySystemdReady is intentionally dormant until every frozen readiness
// gate exists. Keeping the boundary here allows that later activation to remain
// a single lifecycle operation.
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
