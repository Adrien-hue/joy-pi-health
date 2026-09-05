package app

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		args        []string
		environment []string
		wantStatus  int
		wantStdout  string
		wantError   bool
	}{
		{name: "defaults", wantStatus: 0},
		{
			name:        "valid explicit configuration has no runtime side effects",
			args:        []string{"--listen-address=0.0.0.0", "--listen-port=9090", "--allow-non-loopback=true", "--log-level=debug"},
			environment: []string{"PATH=ignored"},
			wantStatus:  0,
		},
		{name: "help", args: []string{"--help"}, wantStatus: 0, wantStdout: config.HelpText},
		{
			name:        "help bypasses environment validation",
			args:        []string{"--help"},
			environment: []string{"JOY_PI_HEALTH_UNKNOWN=value"},
			wantStatus:  0,
			wantStdout:  config.HelpText,
		},
		{name: "help combined with option", args: []string{"--help", "--listen-port=8081"}, wantStatus: 2, wantError: true},
		{name: "command-line error", args: []string{"--listen-port=0"}, wantStatus: 2, wantError: true},
		{name: "environment error", environment: []string{"JOY_PI_HEALTH_UNKNOWN=value"}, wantStatus: 2, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer

			if status := Run(test.args, test.environment, &stdout, &stderr); status != test.wantStatus {
				t.Errorf("Run() status = %d, want %d", status, test.wantStatus)
			}
			if stdout.String() != test.wantStdout {
				t.Errorf("Run() stdout = %q, want %q", stdout.String(), test.wantStdout)
			}
			if !test.wantError {
				if stderr.Len() != 0 {
					t.Errorf("Run() wrote unexpected stderr: %q", stderr.String())
				}
				return
			}
			if !strings.HasPrefix(stderr.String(), "joy-pi-health: configuration error: ") {
				t.Errorf("Run() stderr = %q, want configuration error prefix", stderr.String())
			}
			if !strings.HasSuffix(stderr.String(), "\n") {
				t.Errorf("Run() stderr = %q, want trailing newline", stderr.String())
			}
			if stderr.Len() >= 256 {
				t.Errorf("Run() stderr length = %d, want less than 256", stderr.Len())
			}
		})
	}
}
