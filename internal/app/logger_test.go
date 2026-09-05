package app

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
	"github.com/Adrien-hue/joy-pi-health/internal/observe"
	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

func TestLineLoggerFormatsAndFilters(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 9, 5, 12, 34, 56, 123, time.FixedZone("test", 2*60*60))
	for _, test := range []struct {
		level config.LogLevel
		want  []string
	}{
		{config.LogLevelDebug, []string{"INFO", "WARN", "ERROR"}},
		{config.LogLevelInfo, []string{"INFO", "WARN", "ERROR"}},
		{config.LogLevelWarn, []string{"WARN", "ERROR"}},
		{config.LogLevelError, []string{"ERROR"}},
	} {
		var output bytes.Buffer
		logger := newLineLoggerWith(&output, test.level, func() time.Time { return fixed })
		logger.info("information")
		logger.warn("warning")
		logger.error("failure\ncontinued")
		lines := strings.Split(strings.TrimSpace(output.String()), "\n")
		if len(lines) != len(test.want) {
			t.Fatalf("level %s output = %q", test.level, output.String())
		}
		for index, severity := range test.want {
			if !strings.HasPrefix(lines[index], "2026-09-05T10:34:56.000000123Z "+severity+" ") {
				t.Errorf("line %d = %q", index, lines[index])
			}
		}
		if strings.Contains(output.String(), "failure\ncontinued") {
			t.Error("message created more than one record")
		}
	}
}

func TestLineLoggerFormatsDegradationWithoutPayload(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := newLineLoggerWith(&output, config.LogLevelInfo, time.Now)
	record := observe.DegradationRecord{
		At: time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC), Severity: observe.DegradationWarn,
		Metric: "/memory", Reason: snapshot.IssuePermissionDenied,
	}
	logger.degradation(record)
	if got := output.String(); got != "2026-09-05T10:00:00Z WARN metric unavailable metric=/memory reason=permission_denied\n" {
		t.Fatalf("output = %q", got)
	}
	record.Cleared = true
	record.Severity = observe.DegradationInfo
	logger.degradation(record)
	if strings.Contains(output.String(), "snapshot") || strings.Contains(output.String(), "cause") {
		t.Fatalf("output contains payload detail: %q", output.String())
	}
}
