package observe

import (
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

const (
	degradationReminderInterval = 5 * time.Minute
	maximumDegradationStates    = 204
)

// DegradationSeverity is the small logging surface required by observation.
type DegradationSeverity uint8

const (
	DegradationInfo DegradationSeverity = iota + 1
	DegradationWarn
)

// DegradationRecord contains only stable, non-sensitive degradation identity.
type DegradationRecord struct {
	At       time.Time
	Severity DegradationSeverity
	Metric   string
	Reason   snapshot.IssueCode
	Cleared  bool
}

type degradationEvent struct {
	metric string
	reason snapshot.IssueCode
}

type degradationKey struct {
	metric string
	reason snapshot.IssueCode
}

type degradationEntry struct {
	lastLogged time.Time
}

type degradationReporter interface {
	Report([]degradationEvent) error
}

type noopDegradationReporter struct{}

func (noopDegradationReporter) Report([]degradationEvent) error { return nil }

// DegradationReporter suppresses repeated expected failures without retaining
// raw acquisition errors or historical counters.
type DegradationReporter struct {
	now  func() time.Time
	emit func(DegradationRecord)

	mu     sync.Mutex
	active map[degradationKey]degradationEntry
}

func NewDegradationReporter(emit func(DegradationRecord)) (*DegradationReporter, error) {
	return newDegradationReporter(time.Now, emit)
}

func newDegradationReporter(now func() time.Time, emit func(DegradationRecord)) (*DegradationReporter, error) {
	if now == nil || emit == nil {
		return nil, errors.New("degradation reporter dependencies are required")
	}
	return &DegradationReporter{now: now, emit: emit, active: make(map[degradationKey]degradationEntry)}, nil
}

func (reporter *DegradationReporter) Report(events []degradationEvent) error {
	if len(events) > maximumDegradationStates {
		return errors.New("degradation event limit exceeded")
	}
	now := reporter.now()
	current := make(map[degradationKey]struct{}, len(events))
	for _, event := range events {
		if event.metric == "" || !validIssueCode(event.reason) {
			return errors.New("invalid degradation identity")
		}
		key := degradationKey{metric: event.metric, reason: event.reason}
		if _, duplicate := current[key]; duplicate {
			return errors.New("duplicate degradation identity")
		}
		current[key] = struct{}{}
	}

	reporter.mu.Lock()
	defer reporter.mu.Unlock()
	for _, event := range events {
		key := degradationKey{metric: event.metric, reason: event.reason}
		entry, exists := reporter.active[key]
		if !exists || now.Sub(entry.lastLogged) >= degradationReminderInterval {
			reporter.emit(DegradationRecord{At: now, Severity: DegradationWarn, Metric: key.metric, Reason: key.reason})
			entry.lastLogged = now
		}
		reporter.active[key] = entry
	}

	recovered := make([]degradationKey, 0)
	for key := range reporter.active {
		if _, remains := current[key]; !remains {
			recovered = append(recovered, key)
		}
	}
	sort.Slice(recovered, func(left, right int) bool {
		if recovered[left].metric == recovered[right].metric {
			return recovered[left].reason < recovered[right].reason
		}
		return recovered[left].metric < recovered[right].metric
	})
	for _, key := range recovered {
		reporter.emit(DegradationRecord{At: now, Severity: DegradationInfo, Metric: key.metric, Reason: key.reason, Cleared: true})
		delete(reporter.active, key)
	}
	return nil
}

func validIssueCode(code snapshot.IssueCode) bool {
	switch code {
	case snapshot.IssueUnsupported, snapshot.IssuePermissionDenied, snapshot.IssueTemporarilyUnavailable:
		return true
	default:
		return false
	}
}

func degradationCode(reason availabilityReason) snapshot.IssueCode {
	switch reason {
	case reasonUnsupported:
		return snapshot.IssueUnsupported
	case reasonPermissionDenied:
		return snapshot.IssuePermissionDenied
	case reasonTemporarilyUnavailable:
		return snapshot.IssueTemporarilyUnavailable
	default:
		panic("invalid availability reason")
	}
}
