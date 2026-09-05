package app

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/config"
	"github.com/Adrien-hue/joy-pi-health/internal/observe"
)

type logSeverity uint8

const (
	logInfo logSeverity = iota + 1
	logWarn
	logError
)

type lineLogger struct {
	output  io.Writer
	minimum logSeverity
	now     func() time.Time
	mu      sync.Mutex
}

func newLineLogger(output io.Writer, level config.LogLevel) *lineLogger {
	return newLineLoggerWith(output, level, time.Now)
}

func newLineLoggerWith(output io.Writer, level config.LogLevel, now func() time.Time) *lineLogger {
	minimum := logInfo
	switch level {
	case config.LogLevelWarn:
		minimum = logWarn
	case config.LogLevelError:
		minimum = logError
	}
	return &lineLogger{output: output, minimum: minimum, now: now}
}

func (logger *lineLogger) info(message string)  { logger.writeAt(logger.now(), logInfo, message) }
func (logger *lineLogger) warn(message string)  { logger.writeAt(logger.now(), logWarn, message) }
func (logger *lineLogger) error(message string) { logger.writeAt(logger.now(), logError, message) }

func (logger *lineLogger) degradation(record observe.DegradationRecord) {
	severity := logWarn
	if record.Severity == observe.DegradationInfo {
		severity = logInfo
	}
	message := fmt.Sprintf("metric unavailable metric=%s reason=%s", record.Metric, record.Reason)
	if record.Cleared {
		message = fmt.Sprintf("metric degradation cleared metric=%s reason=%s", record.Metric, record.Reason)
	}
	logger.writeAt(record.At, severity, message)
}

func (logger *lineLogger) writeAt(at time.Time, severity logSeverity, message string) {
	if logger == nil || logger.output == nil || severity < logger.minimum {
		return
	}
	label := "INFO"
	switch severity {
	case logWarn:
		label = "WARN"
	case logError:
		label = "ERROR"
	}
	message = strings.ReplaceAll(strings.TrimSpace(message), "\n", " ")
	message = strings.ReplaceAll(message, "\r", " ")
	logger.mu.Lock()
	_, _ = fmt.Fprintf(logger.output, "%s %s %s\n", at.UTC().Format(time.RFC3339Nano), label, message)
	logger.mu.Unlock()
}

func (logger *lineLogger) errorWriter() io.Writer {
	return lineLogWriter{logger: logger, severity: logError}
}

type lineLogWriter struct {
	logger   *lineLogger
	severity logSeverity
}

func (writer lineLogWriter) Write(data []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line != "" {
			writer.logger.writeAt(writer.logger.now(), writer.severity, line)
		}
	}
	return len(data), nil
}
