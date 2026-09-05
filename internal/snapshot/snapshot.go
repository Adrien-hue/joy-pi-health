// Package snapshot owns the v0.1 snapshot schema, validation, and encoding.
package snapshot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	SchemaVersion     = "1.0"
	MaximumSize       = 64 * 1024
	maximumInterfaces = 64
)

var ErrNoUsefulSnapshot = errors.New("no useful snapshot is available")

type IssueCode string

const (
	IssueUnsupported            IssueCode = "unsupported"
	IssuePermissionDenied       IssueCode = "permission_denied"
	IssueTemporarilyUnavailable IssueCode = "temporarily_unavailable"
)

type Issue struct {
	Path    string
	Code    IssueCode
	Message string
}

type Host struct {
	Hostname string
}

type CPU struct {
	UtilizationPercent *float64
	LogicalCPUCount    *uint64
}

type Load struct {
	OneMinute      *float64
	FiveMinutes    *float64
	FifteenMinutes *float64
}

type Memory struct {
	TotalBytes     *uint64
	AvailableBytes *uint64
	UsedBytes      *uint64
}

type RootFilesystem struct {
	TotalBytes     *uint64
	AvailableBytes *uint64
	UsedBytes      *uint64
}

type Network struct {
	Interfaces []NetworkInterface
}

type NetworkInterface struct {
	Name    string
	State   *string
	RXBytes *uint64
	TXBytes *uint64
}

type RaspberryPi struct {
	SoCTemperatureCelsius              *float64
	ThermalThrottlingActive            *bool
	ThermalThrottlingOccurredSinceBoot *bool
	UndervoltageActive                 *bool
	UndervoltageOccurredSinceBoot      *bool
}

// Snapshot is the logical, current-cycle v0.1 snapshot supplied for finalization.
type Snapshot struct {
	ObservedAt     time.Time
	Host           Host
	CPU            CPU
	Load           Load
	Memory         Memory
	RootFilesystem RootFilesystem
	UptimeSeconds  *uint64
	Network        *Network
	RaspberryPi    RaspberryPi
	Issues         []Issue
}

// Encoded is a validated, immutable JSON snapshot.
type Encoded struct {
	body []byte
}

func (e Encoded) Valid() bool { return len(e.body) != 0 }
func (e Encoded) Len() int    { return len(e.body) }

// WriteTo writes the already-encoded snapshot without exposing its backing bytes.
func (e Encoded) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(e.body)
	return int64(n), err
}

// Encode validates and encodes one logical snapshot exactly once.
func Encode(input Snapshot) (Encoded, error) {
	document, expectedIssues, usable, err := buildDocument(input)
	if err != nil {
		return Encoded{}, err
	}
	if err := validateIssues(input.Issues, expectedIssues); err != nil {
		return Encoded{}, err
	}
	if err := validateFirmwareIssues(input); err != nil {
		return Encoded{}, err
	}
	if usable == 0 {
		return Encoded{}, ErrNoUsefulSnapshot
	}

	document.Issues = make([]wireIssue, len(input.Issues))
	for index, issue := range input.Issues {
		document.Issues[index] = wireIssue(issue)
	}
	body, err := json.Marshal(document)
	if err != nil {
		return Encoded{}, fmt.Errorf("encode snapshot: %w", err)
	}
	if len(body) > MaximumSize {
		return Encoded{}, fmt.Errorf("encoded snapshot is %d bytes, maximum is %d", len(body), MaximumSize)
	}
	return Encoded{body: body}, nil
}

type wireDocument struct {
	SchemaVersion  string             `json:"schema_version"`
	ObservedAt     string             `json:"observed_at"`
	Host           wireHost           `json:"host"`
	CPU            wireCPU            `json:"cpu"`
	Load           wireLoad           `json:"load"`
	Memory         wireMemory         `json:"memory"`
	RootFilesystem wireRootFilesystem `json:"root_filesystem"`
	UptimeSeconds  *uint64            `json:"uptime_seconds"`
	Network        *wireNetwork       `json:"network"`
	RaspberryPi    wireRaspberryPi    `json:"raspberry_pi"`
	Issues         []wireIssue        `json:"issues"`
}

type wireHost struct {
	Hostname string `json:"hostname"`
}

type wireCPU struct {
	UtilizationPercent *float64 `json:"utilization_percent"`
	LogicalCPUCount    *uint64  `json:"logical_cpu_count"`
}

type wireLoad struct {
	OneMinute      *float64 `json:"one_minute"`
	FiveMinutes    *float64 `json:"five_minutes"`
	FifteenMinutes *float64 `json:"fifteen_minutes"`
}

type wireMemory struct {
	TotalBytes     *uint64 `json:"total_bytes"`
	AvailableBytes *uint64 `json:"available_bytes"`
	UsedBytes      *uint64 `json:"used_bytes"`
}

type wireRootFilesystem wireMemory

type wireNetwork struct {
	Interfaces []wireNetworkInterface `json:"interfaces"`
}

type wireNetworkInterface struct {
	Name    string  `json:"name"`
	State   *string `json:"state"`
	RXBytes *uint64 `json:"rx_bytes"`
	TXBytes *uint64 `json:"tx_bytes"`
}

type wireRaspberryPi struct {
	SoCTemperatureCelsius              *float64 `json:"soc_temperature_celsius"`
	ThermalThrottlingActive            *bool    `json:"thermal_throttling_active"`
	ThermalThrottlingOccurredSinceBoot *bool    `json:"thermal_throttling_occurred_since_boot"`
	UndervoltageActive                 *bool    `json:"undervoltage_active"`
	UndervoltageOccurredSinceBoot      *bool    `json:"undervoltage_occurred_since_boot"`
}

type wireIssue struct {
	Path    string    `json:"path"`
	Code    IssueCode `json:"code"`
	Message string    `json:"message"`
}

func buildDocument(input Snapshot) (wireDocument, []string, int, error) {
	if input.ObservedAt.IsZero() {
		return wireDocument{}, nil, 0, errors.New("observed_at is required")
	}
	if input.Host.Hostname == "" || !utf8.ValidString(input.Host.Hostname) {
		return wireDocument{}, nil, 0, errors.New("hostname must be non-empty UTF-8")
	}
	observedAt := input.ObservedAt.UTC().Format(time.RFC3339Nano)
	if _, err := time.Parse(time.RFC3339Nano, observedAt); err != nil {
		return wireDocument{}, nil, 0, fmt.Errorf("observed_at is not RFC 3339: %w", err)
	}

	document := wireDocument{
		SchemaVersion:  SchemaVersion,
		ObservedAt:     observedAt,
		Host:           wireHost{Hostname: input.Host.Hostname},
		CPU:            wireCPU(input.CPU),
		Load:           wireLoad(input.Load),
		Memory:         wireMemory(input.Memory),
		RootFilesystem: wireRootFilesystem(input.RootFilesystem),
		UptimeSeconds:  copyPointer(input.UptimeSeconds),
		RaspberryPi:    wireRaspberryPi(input.RaspberryPi),
	}

	if err := validateFloat(input.CPU.UtilizationPercent, 0, 100, "cpu utilization"); err != nil {
		return wireDocument{}, nil, 0, err
	}
	if input.CPU.LogicalCPUCount != nil && *input.CPU.LogicalCPUCount == 0 {
		return wireDocument{}, nil, 0, errors.New("logical CPU count must be positive")
	}
	for _, candidate := range []struct {
		value *float64
		name  string
	}{
		{input.Load.OneMinute, "one-minute load"},
		{input.Load.FiveMinutes, "five-minute load"},
		{input.Load.FifteenMinutes, "fifteen-minute load"},
	} {
		if err := validateFloat(candidate.value, 0, math.Inf(1), candidate.name); err != nil {
			return wireDocument{}, nil, 0, err
		}
	}
	if err := validateFinite(input.RaspberryPi.SoCTemperatureCelsius, "SoC temperature"); err != nil {
		return wireDocument{}, nil, 0, err
	}
	if err := validateAllOrNone("load", input.Load.OneMinute != nil, input.Load.FiveMinutes != nil, input.Load.FifteenMinutes != nil); err != nil {
		return wireDocument{}, nil, 0, err
	}
	if err := validateMemory("memory", input.Memory.TotalBytes, input.Memory.AvailableBytes, input.Memory.UsedBytes); err != nil {
		return wireDocument{}, nil, 0, err
	}
	if err := validateMemory("root filesystem", input.RootFilesystem.TotalBytes, input.RootFilesystem.AvailableBytes, input.RootFilesystem.UsedBytes); err != nil {
		return wireDocument{}, nil, 0, err
	}
	if err := validateAllOrNone("firmware flags",
		input.RaspberryPi.ThermalThrottlingActive != nil,
		input.RaspberryPi.ThermalThrottlingOccurredSinceBoot != nil,
		input.RaspberryPi.UndervoltageActive != nil,
		input.RaspberryPi.UndervoltageOccurredSinceBoot != nil,
	); err != nil {
		return wireDocument{}, nil, 0, err
	}

	expected := make([]string, 0)
	usable := 0
	countOrIssue(input.UptimeSeconds != nil, "/uptime_seconds", &usable, &expected)
	countOrIssue(input.CPU.LogicalCPUCount != nil, "/cpu/logical_cpu_count", &usable, &expected)
	countGroupOrIssue(input.Load.OneMinute != nil, 3, "/load", &usable, &expected)
	countGroupOrIssue(input.Memory.TotalBytes != nil, 3, "/memory", &usable, &expected)
	countGroupOrIssue(input.RootFilesystem.TotalBytes != nil, 3, "/root_filesystem", &usable, &expected)
	countOrIssue(input.RaspberryPi.SoCTemperatureCelsius != nil, "/raspberry_pi/soc_temperature_celsius", &usable, &expected)
	countOrIssue(input.RaspberryPi.ThermalThrottlingActive != nil, "/raspberry_pi/thermal_throttling_active", &usable, &expected)
	countOrIssue(input.RaspberryPi.ThermalThrottlingOccurredSinceBoot != nil, "/raspberry_pi/thermal_throttling_occurred_since_boot", &usable, &expected)
	countOrIssue(input.RaspberryPi.UndervoltageActive != nil, "/raspberry_pi/undervoltage_active", &usable, &expected)
	countOrIssue(input.RaspberryPi.UndervoltageOccurredSinceBoot != nil, "/raspberry_pi/undervoltage_occurred_since_boot", &usable, &expected)

	if input.Network == nil {
		expected = append(expected, "/network")
	} else {
		if len(input.Network.Interfaces) > maximumInterfaces {
			return wireDocument{}, nil, 0, fmt.Errorf("network has more than %d interfaces", maximumInterfaces)
		}
		document.Network = &wireNetwork{Interfaces: make([]wireNetworkInterface, len(input.Network.Interfaces))}
		previousName := ""
		for index, networkInterface := range input.Network.Interfaces {
			if networkInterface.Name == "" || !utf8.ValidString(networkInterface.Name) {
				return wireDocument{}, nil, 0, fmt.Errorf("network interface %d has an invalid name", index)
			}
			if index != 0 && strings.Compare(previousName, networkInterface.Name) >= 0 {
				return wireDocument{}, nil, 0, errors.New("network interfaces must be uniquely ordered by name")
			}
			if networkInterface.State != nil && (*networkInterface.State == "" || !utf8.ValidString(*networkInterface.State)) {
				return wireDocument{}, nil, 0, fmt.Errorf("network interface %d has an invalid state", index)
			}
			document.Network.Interfaces[index] = wireNetworkInterface{
				Name: networkInterface.Name, State: copyPointer(networkInterface.State),
				RXBytes: copyPointer(networkInterface.RXBytes), TXBytes: copyPointer(networkInterface.TXBytes),
			}
			usable++
			prefix := fmt.Sprintf("/network/interfaces/%d/", index)
			countOrIssue(networkInterface.State != nil, prefix+"state", &usable, &expected)
			countOrIssue(networkInterface.RXBytes != nil, prefix+"rx_bytes", &usable, &expected)
			countOrIssue(networkInterface.TXBytes != nil, prefix+"tx_bytes", &usable, &expected)
			previousName = networkInterface.Name
		}
	}
	countOrIssue(input.CPU.UtilizationPercent != nil, "/cpu/utilization_percent", &usable, &expected)

	document.CPU.UtilizationPercent = copyPointer(input.CPU.UtilizationPercent)
	document.CPU.LogicalCPUCount = copyPointer(input.CPU.LogicalCPUCount)
	document.Load = wireLoad{
		OneMinute: copyPointer(input.Load.OneMinute), FiveMinutes: copyPointer(input.Load.FiveMinutes),
		FifteenMinutes: copyPointer(input.Load.FifteenMinutes),
	}
	document.Memory = wireMemory{
		TotalBytes: copyPointer(input.Memory.TotalBytes), AvailableBytes: copyPointer(input.Memory.AvailableBytes), UsedBytes: copyPointer(input.Memory.UsedBytes),
	}
	document.RootFilesystem = wireRootFilesystem{
		TotalBytes: copyPointer(input.RootFilesystem.TotalBytes), AvailableBytes: copyPointer(input.RootFilesystem.AvailableBytes), UsedBytes: copyPointer(input.RootFilesystem.UsedBytes),
	}
	document.RaspberryPi = wireRaspberryPi{
		SoCTemperatureCelsius:              copyPointer(input.RaspberryPi.SoCTemperatureCelsius),
		ThermalThrottlingActive:            copyPointer(input.RaspberryPi.ThermalThrottlingActive),
		ThermalThrottlingOccurredSinceBoot: copyPointer(input.RaspberryPi.ThermalThrottlingOccurredSinceBoot),
		UndervoltageActive:                 copyPointer(input.RaspberryPi.UndervoltageActive),
		UndervoltageOccurredSinceBoot:      copyPointer(input.RaspberryPi.UndervoltageOccurredSinceBoot),
	}
	return document, expected, usable, nil
}

func validateIssues(issues []Issue, expected []string) error {
	if len(issues) != len(expected) {
		return fmt.Errorf("issues count is %d, want %d", len(issues), len(expected))
	}
	for index, issue := range issues {
		if issue.Path != expected[index] {
			return fmt.Errorf("issue %d path is %q, want %q", index, issue.Path, expected[index])
		}
		switch issue.Code {
		case IssueUnsupported, IssuePermissionDenied, IssueTemporarilyUnavailable:
		default:
			return fmt.Errorf("issue %d has invalid code %q", index, issue.Code)
		}
		if issue.Message == "" || !utf8.ValidString(issue.Message) {
			return fmt.Errorf("issue %d message must be non-empty UTF-8", index)
		}
	}
	return nil
}

func validateFirmwareIssues(input Snapshot) error {
	if input.RaspberryPi.ThermalThrottlingActive != nil {
		return nil
	}
	paths := map[string]struct{}{
		"/raspberry_pi/thermal_throttling_active":              {},
		"/raspberry_pi/thermal_throttling_occurred_since_boot": {},
		"/raspberry_pi/undervoltage_active":                    {},
		"/raspberry_pi/undervoltage_occurred_since_boot":       {},
	}
	var first *Issue
	for index := range input.Issues {
		issue := &input.Issues[index]
		if _, belongs := paths[issue.Path]; !belongs {
			continue
		}
		if first == nil {
			first = issue
			continue
		}
		if issue.Code != first.Code || issue.Message != first.Message {
			return errors.New("coherent firmware issues must share a code and message")
		}
	}
	return nil
}

func validateAllOrNone(name string, present ...bool) error {
	count := 0
	for _, value := range present {
		if value {
			count++
		}
	}
	if count != 0 && count != len(present) {
		return fmt.Errorf("%s must be entirely present or unavailable", name)
	}
	return nil
}

func validateMemory(name string, total, available, used *uint64) error {
	if err := validateAllOrNone(name, total != nil, available != nil, used != nil); err != nil {
		return err
	}
	if total == nil {
		return nil
	}
	if *available > *total || *used != *total-*available {
		return fmt.Errorf("%s byte values are inconsistent", name)
	}
	return nil
}

func validateFinite(value *float64, name string) error {
	if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
		return fmt.Errorf("%s must be finite", name)
	}
	return nil
}

func validateFloat(value *float64, minimum, maximum float64, name string) error {
	if err := validateFinite(value, name); err != nil {
		return err
	}
	if value != nil && (*value < minimum || *value > maximum) {
		return fmt.Errorf("%s is outside its valid range", name)
	}
	return nil
}

func countOrIssue(present bool, path string, usable *int, expected *[]string) {
	if present {
		(*usable)++
		return
	}
	*expected = append(*expected, path)
}

func countGroupOrIssue(present bool, count int, path string, usable *int, expected *[]string) {
	if present {
		*usable += count
		return
	}
	*expected = append(*expected, path)
}

func copyPointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
