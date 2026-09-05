package app

import (
	"errors"
	"testing"
	"time"
)

func TestNotifySystemdReadyWith(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		environment []string
		wantSocket  string
		wantCalls   int
	}{
		{name: "missing socket"},
		{name: "empty socket", environment: []string{"NOTIFY_SOCKET="}},
		{name: "filesystem socket", environment: []string{"NOTIFY_SOCKET=/run/systemd/notify"}, wantSocket: "/run/systemd/notify", wantCalls: 1},
		{name: "abstract socket", environment: []string{"NOTIFY_SOCKET=@systemd-notify"}, wantSocket: "\x00systemd-notify", wantCalls: 1},
		{name: "last effective value", environment: []string{"NOTIFY_SOCKET=/first", "NOTIFY_SOCKET=/second"}, wantSocket: "/second", wantCalls: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			err := notifySystemdReadyWith(test.environment, func(socket string, payload []byte, timeout time.Duration) error {
				calls++
				if socket != test.wantSocket {
					t.Errorf("socket = %q, want %q", socket, test.wantSocket)
				}
				if string(payload) != readyPayload {
					t.Errorf("payload = %q, want %q", payload, readyPayload)
				}
				if timeout != readinessWriteTimeout {
					t.Errorf("timeout = %v, want %v", timeout, readinessWriteTimeout)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("notifySystemdReadyWith() error = %v", err)
			}
			if calls != test.wantCalls {
				t.Errorf("sender calls = %d, want %d", calls, test.wantCalls)
			}
		})
	}
}

func TestNotifySystemdReadyWithPropagatesError(t *testing.T) {
	t.Parallel()

	want := errors.New("send failed")
	err := notifySystemdReadyWith([]string{"NOTIFY_SOCKET=/run/systemd/notify"}, func(string, []byte, time.Duration) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("notifySystemdReadyWith() error = %v, want %v", err, want)
	}
}
