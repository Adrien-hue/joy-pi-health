package observe

import (
	"fmt"
	"testing"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

func TestDegradationReporterSuppressesRemindsAndRecovers(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	records := make([]DegradationRecord, 0)
	reporter, err := newDegradationReporter(func() time.Time { return now }, func(record DegradationRecord) { records = append(records, record) })
	if err != nil {
		t.Fatal(err)
	}
	event := degradationEvent{metric: "/cpu/utilization_percent", reason: snapshot.IssueTemporarilyUnavailable}
	if err := reporter.Report([]degradationEvent{event}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(degradationReminderInterval - time.Nanosecond)
	if err := reporter.Report([]degradationEvent{event}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records before reminder = %#v", records)
	}
	now = now.Add(time.Nanosecond)
	if err := reporter.Report([]degradationEvent{event}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[1].Severity != DegradationWarn {
		t.Fatalf("reminder records = %#v", records)
	}
	if err := reporter.Report(nil); err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 || !records[2].Cleared || records[2].Severity != DegradationInfo {
		t.Fatalf("recovery records = %#v", records)
	}
	if err := reporter.Report([]degradationEvent{event}); err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || records[3].Cleared {
		t.Fatalf("recurrence records = %#v", records)
	}
}

func TestDegradationReporterSeparatesReasonsAndMetrics(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	records := make([]DegradationRecord, 0)
	reporter, _ := newDegradationReporter(func() time.Time { return now }, func(record DegradationRecord) { records = append(records, record) })
	events := []degradationEvent{
		{metric: "/memory", reason: snapshot.IssueUnsupported},
		{metric: "/root_filesystem", reason: snapshot.IssuePermissionDenied},
		{metric: "/cpu/utilization_percent", reason: snapshot.IssueTemporarilyUnavailable},
	}
	if err := reporter.Report(events); err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("records = %#v", records)
	}
	changed := []degradationEvent{{metric: "/memory", reason: snapshot.IssuePermissionDenied}}
	if err := reporter.Report(changed); err != nil {
		t.Fatal(err)
	}
	if len(records) != 7 {
		t.Fatalf("reason-change records = %#v", records)
	}
	if records[3].Reason != snapshot.IssuePermissionDenied || records[3].Cleared {
		t.Fatalf("new reason = %#v", records[3])
	}
}

func TestDegradationReporterEnforcesBoundAndIdentity(t *testing.T) {
	t.Parallel()
	reporter, _ := newDegradationReporter(time.Now, func(DegradationRecord) {})
	events := make([]degradationEvent, maximumDegradationStates)
	for index := range events {
		events[index] = degradationEvent{metric: fmt.Sprintf("metric-%d", index), reason: snapshot.IssueUnsupported}
	}
	if err := reporter.Report(events); err != nil {
		t.Fatal(err)
	}
	if len(reporter.active) != maximumDegradationStates {
		t.Fatalf("active states = %d", len(reporter.active))
	}
	events = append(events, degradationEvent{metric: "one-too-many", reason: snapshot.IssueUnsupported})
	if err := reporter.Report(events); err == nil {
		t.Fatal("oversized degradation state was accepted")
	}
	duplicate := degradationEvent{metric: "/memory", reason: snapshot.IssueUnsupported}
	if err := reporter.Report([]degradationEvent{duplicate, duplicate}); err == nil {
		t.Fatal("duplicate degradation identity was accepted")
	}
	if err := reporter.Report([]degradationEvent{{metric: "/memory", reason: snapshot.IssueCode("internal_error")}}); err == nil {
		t.Fatal("internal_error degradation was accepted")
	}
}
