package snapshot

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEncodeCompleteSnapshotPreservesExactIntegers(t *testing.T) {
	input := completeSnapshot()
	large := uint64(9007199254740993)
	maximum := uint64(math.MaxUint64)
	input.Network.Interfaces[0].RXBytes = &large
	input.Network.Interfaces[0].TXBytes = &maximum

	encoded, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	var body bytes.Buffer
	if _, err := encoded.WriteTo(&body); err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	if !bytes.Contains(body.Bytes(), []byte(`"rx_bytes":9007199254740993`)) {
		t.Errorf("encoded snapshot does not preserve integer above 2^53: %s", body.Bytes())
	}
	if !bytes.Contains(body.Bytes(), []byte(`"tx_bytes":18446744073709551615`)) {
		t.Errorf("encoded snapshot does not preserve MaxUint64: %s", body.Bytes())
	}

	decoder := json.NewDecoder(bytes.NewReader(body.Bytes()))
	decoder.UseNumber()
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got := document["schema_version"]; got != SchemaVersion {
		t.Errorf("schema_version = %v, want %q", got, SchemaVersion)
	}
	if got := document["observed_at"]; got != "2026-09-05T10:34:56.123Z" {
		t.Errorf("observed_at = %v, want UTC timestamp", got)
	}
}

func TestEncodePartialSnapshotPreservesIssueOrderAndNulls(t *testing.T) {
	input := completeSnapshot()
	input.UptimeSeconds = nil
	input.Memory = Memory{}
	input.Network.Interfaces[0].State = nil
	input.CPU.UtilizationPercent = nil
	input.Issues = []Issue{
		issue("/uptime_seconds"),
		issue("/memory"),
		issue("/network/interfaces/0/state"),
		issue("/cpu/utilization_percent"),
	}

	encoded, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	var body bytes.Buffer
	_, _ = encoded.WriteTo(&body)
	if !bytes.Contains(body.Bytes(), []byte(`"uptime_seconds":null`)) ||
		!bytes.Contains(body.Bytes(), []byte(`"memory":{"total_bytes":null`)) {
		t.Errorf("partial snapshot does not contain required nulls: %s", body.Bytes())
	}
	positions := make([]int, len(input.Issues))
	for index, issue := range input.Issues {
		positions[index] = bytes.Index(body.Bytes(), []byte(`"path":"`+issue.Path+`"`))
		if positions[index] < 0 || index > 0 && positions[index] <= positions[index-1] {
			t.Fatalf("issues are not encoded in supplied deterministic order: %s", body.Bytes())
		}
	}
}

func TestEncodeCopiesMutableInput(t *testing.T) {
	input := completeSnapshot()
	encoded, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	input.Network.Interfaces[0].Name = "changed"
	*input.Memory.TotalBytes = 1

	var body bytes.Buffer
	_, _ = encoded.WriteTo(&body)
	if bytes.Contains(body.Bytes(), []byte("changed")) || !bytes.Contains(body.Bytes(), []byte(`"total_bytes":1000`)) {
		t.Errorf("encoded result changed with caller-owned input: %s", body.Bytes())
	}
}

func TestEncodeRejectsInvalidSnapshots(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Snapshot)
	}{
		{name: "zero time", change: func(s *Snapshot) { s.ObservedAt = time.Time{} }},
		{name: "unrepresentable RFC 3339 time", change: func(s *Snapshot) { s.ObservedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) }},
		{name: "empty hostname", change: func(s *Snapshot) { s.Host.Hostname = "" }},
		{name: "non-finite float", change: func(s *Snapshot) { value := math.NaN(); s.CPU.UtilizationPercent = &value }},
		{name: "inconsistent memory", change: func(s *Snapshot) { *s.Memory.UsedBytes = 1 }},
		{name: "partial firmware group", change: func(s *Snapshot) { s.RaspberryPi.UndervoltageActive = nil }},
		{name: "unsorted interfaces", change: func(s *Snapshot) {
			s.Network.Interfaces = append(s.Network.Interfaces, NetworkInterface{Name: "aaa", State: pointer("up"), RXBytes: pointer(uint64(1)), TXBytes: pointer(uint64(1))})
		}},
		{name: "too many interfaces", change: func(s *Snapshot) {
			s.Network.Interfaces = make([]NetworkInterface, maximumInterfaces+1)
			for index := range s.Network.Interfaces {
				s.Network.Interfaces[index] = NetworkInterface{Name: fmt.Sprintf("eth%03d", index), State: pointer("up"), RXBytes: pointer(uint64(1)), TXBytes: pointer(uint64(1))}
			}
		}},
		{name: "invalid issue code", change: func(s *Snapshot) {
			s.CPU.UtilizationPercent = nil
			s.Issues = []Issue{{Path: "/cpu/utilization_percent", Code: IssueCode("internal_error"), Message: "bad"}}
		}},
		{name: "wrong issue order", change: func(s *Snapshot) {
			s.UptimeSeconds = nil
			s.CPU.UtilizationPercent = nil
			s.Issues = []Issue{issue("/cpu/utilization_percent"), issue("/uptime_seconds")}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := completeSnapshot()
			test.change(&input)
			if _, err := Encode(input); err == nil {
				t.Fatal("Encode() error = nil, want validation error")
			}
		})
	}
}

func TestEncodeReportsNoUsefulSnapshot(t *testing.T) {
	input := Snapshot{
		ObservedAt: time.Date(2026, 9, 5, 12, 34, 56, 0, time.UTC),
		Host:       Host{Hostname: "raspberrypi"},
		Issues: []Issue{
			issue("/uptime_seconds"), issue("/cpu/logical_cpu_count"), issue("/load"),
			issue("/memory"), issue("/root_filesystem"),
			issue("/raspberry_pi/soc_temperature_celsius"),
			issue("/raspberry_pi/thermal_throttling_active"),
			issue("/raspberry_pi/thermal_throttling_occurred_since_boot"),
			issue("/raspberry_pi/undervoltage_active"),
			issue("/raspberry_pi/undervoltage_occurred_since_boot"),
			issue("/network"), issue("/cpu/utilization_percent"),
		},
	}
	_, err := Encode(input)
	if !errors.Is(err, ErrNoUsefulSnapshot) {
		t.Fatalf("Encode() error = %v, want ErrNoUsefulSnapshot", err)
	}
}

func TestEncodeEnforcesMaximumSize(t *testing.T) {
	input := completeSnapshot()
	input.CPU.UtilizationPercent = nil
	input.Issues = []Issue{{Path: "/cpu/utilization_percent", Code: IssueTemporarilyUnavailable, Message: "x"}}
	baseline, err := Encode(input)
	if err != nil {
		t.Fatalf("baseline Encode() error = %v", err)
	}
	input.Issues[0].Message = strings.Repeat("x", MaximumSize-baseline.Len()+1)
	atLimit, err := Encode(input)
	if err != nil {
		t.Fatalf("Encode() at size limit error = %v", err)
	}
	if atLimit.Len() != MaximumSize {
		t.Fatalf("encoded size = %d, want %d", atLimit.Len(), MaximumSize)
	}
	input.Issues[0].Message += "x"
	if _, err := Encode(input); err == nil || errors.Is(err, ErrNoUsefulSnapshot) {
		t.Fatalf("Encode() error = %v, want size error", err)
	}
}

func TestLargeIntegerFixtureUsesExactJSONNumbers(t *testing.T) {
	body, err := os.ReadFile("testdata/large-integers.json")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var values map[string]json.Number
	if err := decoder.Decode(&values); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if values["above_2_53"].String() != "9007199254740993" || values["max_uint64"].String() != "18446744073709551615" {
		t.Fatalf("fixture values lost exact decimal form: %v", values)
	}
}

func completeSnapshot() Snapshot {
	return Snapshot{
		ObservedAt:     time.Date(2026, 9, 5, 12, 34, 56, 123000000, time.FixedZone("test", 2*60*60)),
		Host:           Host{Hostname: "raspberrypi"},
		CPU:            CPU{UtilizationPercent: pointer(12.5), LogicalCPUCount: pointer(uint64(4))},
		Load:           Load{OneMinute: pointer(0.12), FiveMinutes: pointer(0.18), FifteenMinutes: pointer(0.21)},
		Memory:         Memory{TotalBytes: pointer(uint64(1000)), AvailableBytes: pointer(uint64(600)), UsedBytes: pointer(uint64(400))},
		RootFilesystem: RootFilesystem{TotalBytes: pointer(uint64(2000)), AvailableBytes: pointer(uint64(1200)), UsedBytes: pointer(uint64(800))},
		UptimeSeconds:  pointer(uint64(86400)),
		Network: &Network{Interfaces: []NetworkInterface{{
			Name: "eth0", State: pointer("up"), RXBytes: pointer(uint64(10)), TXBytes: pointer(uint64(20)),
		}}},
		RaspberryPi: RaspberryPi{
			SoCTemperatureCelsius: pointer(48.25), ThermalThrottlingActive: pointer(false),
			ThermalThrottlingOccurredSinceBoot: pointer(false), UndervoltageActive: pointer(false),
			UndervoltageOccurredSinceBoot: pointer(false),
		},
		Issues: []Issue{},
	}
}

func issue(path string) Issue {
	return Issue{Path: path, Code: IssueTemporarilyUnavailable, Message: "The metric is temporarily unavailable."}
}

func pointer[T any](value T) *T { return &value }
