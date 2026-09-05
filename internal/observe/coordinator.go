package observe

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

const (
	collectionCycleTimeout = 500 * time.Millisecond
	collectorDomainTimeout = 100 * time.Millisecond
)

var errCollectionDeadline = errors.New("collection deadline expired")

type cycleCollectors interface {
	CollectHost(context.Context) hostObservation
	CollectCPULoad(context.Context) cpuLoadObservation
	CollectMemory(context.Context) outcome[memoryObservation]
	CollectRootFilesystem(context.Context) outcome[filesystemObservation]
	CollectNetwork(context.Context) outcome[networkObservation]
}

type productionCollectors struct {
	source platform.Source
}

func (collectors productionCollectors) CollectHost(ctx context.Context) hostObservation {
	return collectHost(ctx, collectors.source)
}

func (collectors productionCollectors) CollectCPULoad(ctx context.Context) cpuLoadObservation {
	return collectCPULoad(ctx, collectors.source)
}

func (collectors productionCollectors) CollectMemory(ctx context.Context) outcome[memoryObservation] {
	return collectMemory(ctx, collectors.source)
}

func (collectors productionCollectors) CollectRootFilesystem(ctx context.Context) outcome[filesystemObservation] {
	return collectRootFilesystem(ctx, collectors.source)
}

func (collectors productionCollectors) CollectNetwork(ctx context.Context) outcome[networkObservation] {
	return collectNetwork(ctx, collectors.source)
}

type timeoutFactory func(context.Context, time.Duration) (context.Context, context.CancelFunc)
type snapshotEncoder func(snapshot.Snapshot) (snapshot.Encoded, error)

type coordinatorDependencies struct {
	collectors  cycleCollectors
	now         func() time.Time
	withTimeout timeoutFactory
	encode      snapshotEncoder
}

// Coordinator owns at most one current generic snapshot collection cycle.
type Coordinator struct {
	lifecycle   context.Context
	collectors  cycleCollectors
	now         func() time.Time
	withTimeout timeoutFactory
	encode      snapshotEncoder
	fatal       func(error)

	mu        sync.Mutex
	active    *collectionCycle
	failed    error
	fatalOnce sync.Once
}

type collectionCycle struct {
	done    chan struct{}
	encoded snapshot.Encoded
	err     error
}

// NewCoordinator creates the production generic observation coordinator.
func NewCoordinator(lifecycle context.Context, source platform.Source, fatal func(error)) (*Coordinator, error) {
	if source == nil {
		return nil, errors.New("platform source is required")
	}
	return newCoordinator(lifecycle, fatal, coordinatorDependencies{
		collectors:  productionCollectors{source: source},
		now:         time.Now,
		withTimeout: context.WithTimeout,
		encode:      snapshot.Encode,
	})
}

func newCoordinator(lifecycle context.Context, fatal func(error), dependencies coordinatorDependencies) (*Coordinator, error) {
	switch {
	case lifecycle == nil:
		return nil, errors.New("lifecycle context is required")
	case fatal == nil:
		return nil, errors.New("fatal notifier is required")
	case dependencies.collectors == nil:
		return nil, errors.New("collectors are required")
	case dependencies.now == nil:
		return nil, errors.New("wall clock is required")
	case dependencies.withTimeout == nil:
		return nil, errors.New("timeout factory is required")
	case dependencies.encode == nil:
		return nil, errors.New("snapshot encoder is required")
	}
	return &Coordinator{
		lifecycle: lifecycle, collectors: dependencies.collectors, now: dependencies.now,
		withTimeout: dependencies.withTimeout, encode: dependencies.encode, fatal: fatal,
	}, nil
}

// Snapshot joins or starts the current collection cycle. Request cancellation
// detaches only this caller and never cancels shared collection work.
func (coordinator *Coordinator) Snapshot(request context.Context) (snapshot.Encoded, error) {
	if request == nil {
		return snapshot.Encoded{}, coordinator.fail(internalDefect(errors.New("request context is nil")))
	}
	if err := request.Err(); err != nil {
		return snapshot.Encoded{}, err
	}

	coordinator.mu.Lock()
	if coordinator.failed != nil {
		err := coordinator.failed
		coordinator.mu.Unlock()
		return snapshot.Encoded{}, err
	}
	cycle := coordinator.active
	if cycle == nil {
		cycle = &collectionCycle{done: make(chan struct{})}
		coordinator.active = cycle
		go coordinator.run(cycle)
	}
	coordinator.mu.Unlock()

	select {
	case <-request.Done():
		return snapshot.Encoded{}, request.Err()
	case <-cycle.done:
		return cycle.encoded, cycle.err
	}
}

func (coordinator *Coordinator) run(cycle *collectionCycle) {
	var encoded snapshot.Encoded
	var resultErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			resultErr = internalDefect(fmt.Errorf("collection cycle panic: %v", recovered))
		}

		coordinator.mu.Lock()
		if isInternalDefect(resultErr) && coordinator.failed == nil {
			coordinator.failed = resultErr
		}
		cycle.encoded = encoded
		cycle.err = resultErr
		if coordinator.active == cycle {
			coordinator.active = nil
		}
		coordinator.mu.Unlock()
		defer close(cycle.done)

		if isInternalDefect(resultErr) {
			coordinator.fatalOnce.Do(func() { coordinator.fatal(resultErr) })
		}
	}()
	encoded, resultErr = coordinator.collect()
}

func (coordinator *Coordinator) fail(err error) error {
	coordinator.mu.Lock()
	if coordinator.failed == nil {
		coordinator.failed = err
	}
	failed := coordinator.failed
	coordinator.mu.Unlock()
	coordinator.fatalOnce.Do(func() { coordinator.fatal(failed) })
	return failed
}

type internalDefectError struct {
	cause error
}

func (failure *internalDefectError) Error() string { return failure.cause.Error() }
func (failure *internalDefectError) Unwrap() error { return failure.cause }

func internalDefect(err error) error {
	if err == nil {
		err = errors.New("unspecified internal defect")
	}
	return &internalDefectError{cause: err}
}

func isInternalDefect(err error) bool {
	var target *internalDefectError
	return errors.As(err, &target)
}

type cycleObservations struct {
	host    hostObservation
	cpuLoad cpuLoadObservation
	memory  outcome[memoryObservation]
	root    outcome[filesystemObservation]
	network outcome[networkObservation]
}

func (coordinator *Coordinator) collect() (snapshot.Encoded, error) {
	cycleContext, cancelCycle := coordinator.withTimeout(coordinator.lifecycle, collectionCycleTimeout)
	defer cancelCycle()

	observations := unavailableCycle(errCollectionDeadline)

	if coordinator.lifecycle.Err() != nil {
		return snapshot.Encoded{}, coordinator.lifecycle.Err()
	}
	value, expired := runDomain(cycleContext, coordinator.withTimeout, coordinator.collectors.CollectHost)
	if err := validateHost(value); err != nil {
		return snapshot.Encoded{}, internalDefect(err)
	}
	if !expired {
		observations.host = value
	}

	if coordinator.lifecycle.Err() != nil {
		return snapshot.Encoded{}, coordinator.lifecycle.Err()
	}
	if cycleContext.Err() == nil {
		value, expired := runDomain(cycleContext, coordinator.withTimeout, coordinator.collectors.CollectCPULoad)
		if err := validateCPULoad(value); err != nil {
			return snapshot.Encoded{}, internalDefect(err)
		}
		if !expired {
			observations.cpuLoad = value
		}
	}

	if coordinator.lifecycle.Err() != nil {
		return snapshot.Encoded{}, coordinator.lifecycle.Err()
	}
	if cycleContext.Err() == nil {
		value, expired := runDomain(cycleContext, coordinator.withTimeout, coordinator.collectors.CollectMemory)
		if err := validateMemoryOutcome(value); err != nil {
			return snapshot.Encoded{}, internalDefect(err)
		}
		if !expired {
			observations.memory = value
		}
	}

	if coordinator.lifecycle.Err() != nil {
		return snapshot.Encoded{}, coordinator.lifecycle.Err()
	}
	if cycleContext.Err() == nil {
		value, expired := runDomain(cycleContext, coordinator.withTimeout, coordinator.collectors.CollectRootFilesystem)
		if err := validateFilesystemOutcome(value); err != nil {
			return snapshot.Encoded{}, internalDefect(err)
		}
		if !expired {
			observations.root = value
		}
	}

	// Raspberry Pi collection occupies this fixed position in the cycle. Its
	// fields remain intentionally unavailable until the production collectors exist.

	if coordinator.lifecycle.Err() != nil {
		return snapshot.Encoded{}, coordinator.lifecycle.Err()
	}
	if cycleContext.Err() == nil {
		value, expired := runDomain(cycleContext, coordinator.withTimeout, coordinator.collectors.CollectNetwork)
		if err := validateNetwork(value); err != nil {
			return snapshot.Encoded{}, internalDefect(err)
		}
		if !expired {
			observations.network = value
		}
	}

	// CPU utilization is copied last in the frozen order. It remains
	// intentionally unavailable until the background observer exists.
	if coordinator.lifecycle.Err() != nil {
		return snapshot.Encoded{}, coordinator.lifecycle.Err()
	}
	return coordinator.finalize(observations)
}

func runDomain[T any](parent context.Context, withTimeout timeoutFactory, collect func(context.Context) T) (T, bool) {
	domainContext, cancelDomain := withTimeout(parent, collectorDomainTimeout)
	defer cancelDomain()
	value := collect(domainContext)
	return value, domainContext.Err() != nil
}

func unavailableCycle(cause error) cycleObservations {
	return cycleObservations{
		host: hostObservation{
			hostname: unavailable[string](reasonTemporarilyUnavailable, cause),
			uptime:   unavailable[uint64](reasonTemporarilyUnavailable, cause),
		},
		cpuLoad: cpuLoadObservation{
			logicalCPUCount: unavailable[uint64](reasonTemporarilyUnavailable, cause),
			load:            unavailable[loadObservation](reasonTemporarilyUnavailable, cause),
		},
		memory:  unavailable[memoryObservation](reasonTemporarilyUnavailable, cause),
		root:    unavailable[filesystemObservation](reasonTemporarilyUnavailable, cause),
		network: unavailable[networkObservation](reasonTemporarilyUnavailable, cause),
	}
}

func validateHost(value hostObservation) error {
	if err := validateOutcome(value.hostname); err != nil {
		return fmt.Errorf("hostname outcome: %w", err)
	}
	if value.hostname.state == statePresent && (value.hostname.value == "" || !utf8.ValidString(value.hostname.value)) {
		return errors.New("present hostname is invalid")
	}
	if err := validateOutcome(value.uptime); err != nil {
		return fmt.Errorf("uptime outcome: %w", err)
	}
	return nil
}

func validateCPULoad(value cpuLoadObservation) error {
	if err := validateOutcome(value.logicalCPUCount); err != nil {
		return fmt.Errorf("logical CPU count outcome: %w", err)
	}
	if value.logicalCPUCount.state == statePresent && value.logicalCPUCount.value == 0 {
		return errors.New("present logical CPU count is zero")
	}
	if err := validateOutcome(value.load); err != nil {
		return fmt.Errorf("load outcome: %w", err)
	}
	if value.load.state == statePresent {
		loads := []float64{value.load.value.oneMinute, value.load.value.fiveMinutes, value.load.value.fifteenMinutes}
		for _, load := range loads {
			if math.IsNaN(load) || math.IsInf(load, 0) || load < 0 {
				return errors.New("present load observation is invalid")
			}
		}
	}
	return nil
}

func validateMemoryOutcome(value outcome[memoryObservation]) error {
	if err := validateOutcome(value); err != nil {
		return err
	}
	if value.state == statePresent && (value.value.availableBytes > value.value.totalBytes || value.value.usedBytes != value.value.totalBytes-value.value.availableBytes) {
		return errors.New("present memory observation is inconsistent")
	}
	return nil
}

func validateFilesystemOutcome(value outcome[filesystemObservation]) error {
	if err := validateOutcome(value); err != nil {
		return err
	}
	if value.state == statePresent && (value.value.availableBytes > value.value.totalBytes || value.value.usedBytes != value.value.totalBytes-value.value.availableBytes) {
		return errors.New("present filesystem observation is inconsistent")
	}
	return nil
}

func validateNetwork(value outcome[networkObservation]) error {
	if err := validateOutcome(value); err != nil {
		return err
	}
	if value.state != statePresent {
		return nil
	}
	if len(value.value.interfaces) > maximumInterfaces {
		return errors.New("present network observation exceeds the interface limit")
	}
	previousName := ""
	for index, networkInterface := range value.value.interfaces {
		if networkInterface.name == "" || !utf8.ValidString(networkInterface.name) || index != 0 && strings.Compare(previousName, networkInterface.name) >= 0 {
			return errors.New("present network interfaces are not uniquely ordered by valid name")
		}
		if err := validateOutcome(networkInterface.state); err != nil {
			return fmt.Errorf("interface %q state outcome: %w", networkInterface.name, err)
		}
		if err := validateOutcome(networkInterface.rxBytes); err != nil {
			return fmt.Errorf("interface %q receive outcome: %w", networkInterface.name, err)
		}
		if err := validateOutcome(networkInterface.txBytes); err != nil {
			return fmt.Errorf("interface %q transmit outcome: %w", networkInterface.name, err)
		}
		if networkInterface.state.state == statePresent {
			switch networkInterface.state.value {
			case "up", "down", "unknown":
			default:
				return fmt.Errorf("interface %q has an invalid present state", networkInterface.name)
			}
		}
		previousName = networkInterface.name
	}
	return nil
}

func (coordinator *Coordinator) finalize(observations cycleObservations) (snapshot.Encoded, error) {
	document := snapshot.Snapshot{Issues: make([]snapshot.Issue, 0, 12)}

	if observations.host.hostname.state == statePresent {
		document.Host.Hostname = observations.host.hostname.value
	}
	setValue(&document.UptimeSeconds, observations.host.uptime, "/uptime_seconds", &document.Issues)
	setValue(&document.CPU.LogicalCPUCount, observations.cpuLoad.logicalCPUCount, "/cpu/logical_cpu_count", &document.Issues)
	setLoad(&document, observations.cpuLoad.load)
	setMemory(&document.Memory, observations.memory, "/memory", &document.Issues)
	setRootFilesystem(&document.RootFilesystem, observations.root, "/root_filesystem", &document.Issues)

	appendUnavailableIssue(&document.Issues, "/raspberry_pi/soc_temperature_celsius", reasonTemporarilyUnavailable)
	appendUnavailableIssue(&document.Issues, "/raspberry_pi/thermal_throttling_active", reasonTemporarilyUnavailable)
	appendUnavailableIssue(&document.Issues, "/raspberry_pi/thermal_throttling_occurred_since_boot", reasonTemporarilyUnavailable)
	appendUnavailableIssue(&document.Issues, "/raspberry_pi/undervoltage_active", reasonTemporarilyUnavailable)
	appendUnavailableIssue(&document.Issues, "/raspberry_pi/undervoltage_occurred_since_boot", reasonTemporarilyUnavailable)

	setNetwork(&document, observations.network)
	appendUnavailableIssue(&document.Issues, "/cpu/utilization_percent", reasonTemporarilyUnavailable)

	if observations.host.hostname.state != statePresent {
		return snapshot.Encoded{}, snapshot.ErrNoUsefulSnapshot
	}
	document.ObservedAt = coordinator.now().UTC()
	encoded, err := coordinator.encode(document)
	if err == nil || errors.Is(err, snapshot.ErrNoUsefulSnapshot) {
		return encoded, err
	}
	return snapshot.Encoded{}, internalDefect(fmt.Errorf("finalize snapshot: %w", err))
}

func setLoad(document *snapshot.Snapshot, result outcome[loadObservation]) {
	if result.state == statePresent {
		document.Load.OneMinute = pointer(result.value.oneMinute)
		document.Load.FiveMinutes = pointer(result.value.fiveMinutes)
		document.Load.FifteenMinutes = pointer(result.value.fifteenMinutes)
		return
	}
	appendUnavailableIssue(&document.Issues, "/load", result.reason)
}

func setMemory(target *snapshot.Memory, result outcome[memoryObservation], path string, issues *[]snapshot.Issue) {
	if result.state == statePresent {
		target.TotalBytes = pointer(result.value.totalBytes)
		target.AvailableBytes = pointer(result.value.availableBytes)
		target.UsedBytes = pointer(result.value.usedBytes)
		return
	}
	appendUnavailableIssue(issues, path, result.reason)
}

func setRootFilesystem(target *snapshot.RootFilesystem, result outcome[filesystemObservation], path string, issues *[]snapshot.Issue) {
	if result.state == statePresent {
		target.TotalBytes = pointer(result.value.totalBytes)
		target.AvailableBytes = pointer(result.value.availableBytes)
		target.UsedBytes = pointer(result.value.usedBytes)
		return
	}
	appendUnavailableIssue(issues, path, result.reason)
}

func setNetwork(document *snapshot.Snapshot, result outcome[networkObservation]) {
	if result.state != statePresent {
		appendUnavailableIssue(&document.Issues, "/network", result.reason)
		return
	}
	document.Network = &snapshot.Network{Interfaces: make([]snapshot.NetworkInterface, len(result.value.interfaces))}
	for index, observed := range result.value.interfaces {
		target := &document.Network.Interfaces[index]
		target.Name = observed.name
		prefix := fmt.Sprintf("/network/interfaces/%d/", index)
		setValue(&target.State, observed.state, prefix+"state", &document.Issues)
		setValue(&target.RXBytes, observed.rxBytes, prefix+"rx_bytes", &document.Issues)
		setValue(&target.TXBytes, observed.txBytes, prefix+"tx_bytes", &document.Issues)
	}
}

func setValue[T any](target **T, result outcome[T], path string, issues *[]snapshot.Issue) {
	if result.state == statePresent {
		*target = pointer(result.value)
		return
	}
	appendUnavailableIssue(issues, path, result.reason)
}

func appendUnavailableIssue(issues *[]snapshot.Issue, path string, reason availabilityReason) {
	issue := snapshot.Issue{Path: path}
	switch reason {
	case reasonUnsupported:
		issue.Code = snapshot.IssueUnsupported
		issue.Message = "The metric is unsupported on this platform."
	case reasonPermissionDenied:
		issue.Code = snapshot.IssuePermissionDenied
		issue.Message = "The metric is not accessible to the service account."
	case reasonTemporarilyUnavailable:
		issue.Code = snapshot.IssueTemporarilyUnavailable
		issue.Message = "The metric is temporarily unavailable."
	default:
		panic("invalid availability reason")
	}
	*issues = append(*issues, issue)
}

func pointer[T any](value T) *T {
	return &value
}
