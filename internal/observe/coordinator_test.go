package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

type collectorStub struct {
	mu      sync.Mutex
	calls   []string
	host    hostObservation
	cpuLoad cpuLoadObservation
	memory  outcome[memoryObservation]
	root    outcome[filesystemObservation]
	network outcome[networkObservation]
	hostFn  func(context.Context) hostObservation
	memFn   func(context.Context) outcome[memoryObservation]
}

func completeCollectorStub() *collectorStub {
	return &collectorStub{
		host: hostObservation{hostname: present("raspberrypi"), uptime: present(uint64(123))},
		cpuLoad: cpuLoadObservation{
			logicalCPUCount: present(uint64(4)),
			load: present(loadObservation{
				oneMinute: 0.1, fiveMinutes: 0.2, fifteenMinutes: 0.3,
			}),
		},
		memory: present(memoryObservation{totalBytes: 1024, availableBytes: 256, usedBytes: 768}),
		root:   present(filesystemObservation{totalBytes: 4096, availableBytes: 1024, usedBytes: 3072}),
		network: present(networkObservation{interfaces: []networkInterfaceObservation{{
			name: "eth0", state: present("up"), rxBytes: present(uint64(1<<53 + 1)), txBytes: present(^uint64(0)),
		}}}),
	}
}

func (collectors *collectorStub) record(name string) {
	collectors.mu.Lock()
	collectors.calls = append(collectors.calls, name)
	collectors.mu.Unlock()
}

func (collectors *collectorStub) CollectHost(ctx context.Context) hostObservation {
	collectors.record("host")
	if collectors.hostFn != nil {
		return collectors.hostFn(ctx)
	}
	return collectors.host
}

func (collectors *collectorStub) CollectCPULoad(context.Context) cpuLoadObservation {
	collectors.record("cpu_load")
	return collectors.cpuLoad
}

func (collectors *collectorStub) CollectMemory(ctx context.Context) outcome[memoryObservation] {
	collectors.record("memory")
	if collectors.memFn != nil {
		return collectors.memFn(ctx)
	}
	return collectors.memory
}

func (collectors *collectorStub) CollectRootFilesystem(context.Context) outcome[filesystemObservation] {
	collectors.record("root")
	return collectors.root
}

func (collectors *collectorStub) CollectNetwork(context.Context) outcome[networkObservation] {
	collectors.record("network")
	return collectors.network
}

func (collectors *collectorStub) recordedCalls() []string {
	collectors.mu.Lock()
	defer collectors.mu.Unlock()
	return append([]string(nil), collectors.calls...)
}

func TestCoordinatorBuildsGenericPartialSnapshot(t *testing.T) {
	t.Parallel()
	fixedTime := time.Date(2026, 9, 5, 12, 34, 56, 123000000, time.FixedZone("test", 2*60*60))
	collectors := completeCollectorStub()
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, func() time.Time { return fixedTime }, context.WithTimeout, snapshot.Encode)

	encoded, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	document := decodeDocument(t, encoded)
	if got := document["observed_at"]; got != "2026-09-05T10:34:56.123Z" {
		t.Errorf("observed_at = %v", got)
	}
	issues := document["issues"].([]any)
	wantPaths := []string{
		"/raspberry_pi/soc_temperature_celsius",
		"/raspberry_pi/thermal_throttling_active",
		"/raspberry_pi/thermal_throttling_occurred_since_boot",
		"/raspberry_pi/undervoltage_active",
		"/raspberry_pi/undervoltage_occurred_since_boot",
		"/cpu/utilization_percent",
	}
	if len(issues) != len(wantPaths) {
		t.Fatalf("issues = %#v", issues)
	}
	for index, wantPath := range wantPaths {
		issue := issues[index].(map[string]any)
		if issue["path"] != wantPath || issue["code"] != "temporarily_unavailable" || issue["message"] != "The metric is temporarily unavailable." {
			t.Errorf("issue %d = %#v", index, issue)
		}
	}
	cpu := document["cpu"].(map[string]any)
	if cpu["utilization_percent"] != nil || cpu["logical_cpu_count"] != float64(4) {
		t.Errorf("cpu = %#v", cpu)
	}
	pi := document["raspberry_pi"].(map[string]any)
	for field, value := range pi {
		if value != nil {
			t.Errorf("raspberry_pi.%s = %v; want null", field, value)
		}
	}
}

func TestCoordinatorMapsExpectedReasonsInFrozenOrder(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	collectors.cpuLoad.logicalCPUCount = unavailable[uint64](reasonPermissionDenied, errors.New("denied"))
	collectors.cpuLoad.load = unavailable[loadObservation](reasonTemporarilyUnavailable, errors.New("bad data"))
	collectors.memory = unavailable[memoryObservation](reasonPermissionDenied, errors.New("denied"))
	collectors.root = unavailable[filesystemObservation](reasonUnsupported, errors.New("missing"))
	collectors.network = unavailable[networkObservation](reasonPermissionDenied, errors.New("denied"))
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, snapshot.Encode)

	encoded, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	issues := decodeDocument(t, encoded)["issues"].([]any)
	want := []struct{ path, code, message string }{
		{"/cpu/logical_cpu_count", "permission_denied", "The metric is not accessible to the service account."},
		{"/load", "temporarily_unavailable", "The metric is temporarily unavailable."},
		{"/memory", "permission_denied", "The metric is not accessible to the service account."},
		{"/root_filesystem", "unsupported", "The metric is unsupported on this platform."},
		{"/raspberry_pi/soc_temperature_celsius", "temporarily_unavailable", "The metric is temporarily unavailable."},
		{"/raspberry_pi/thermal_throttling_active", "temporarily_unavailable", "The metric is temporarily unavailable."},
		{"/raspberry_pi/thermal_throttling_occurred_since_boot", "temporarily_unavailable", "The metric is temporarily unavailable."},
		{"/raspberry_pi/undervoltage_active", "temporarily_unavailable", "The metric is temporarily unavailable."},
		{"/raspberry_pi/undervoltage_occurred_since_boot", "temporarily_unavailable", "The metric is temporarily unavailable."},
		{"/network", "permission_denied", "The metric is not accessible to the service account."},
		{"/cpu/utilization_percent", "temporarily_unavailable", "The metric is temporarily unavailable."},
	}
	if len(issues) != len(want) {
		t.Fatalf("issue count = %d; want %d", len(issues), len(want))
	}
	for index, expected := range want {
		issue := issues[index].(map[string]any)
		if issue["path"] != expected.path || issue["code"] != expected.code || issue["message"] != expected.message {
			t.Errorf("issue %d = %#v; want %#v", index, issue, expected)
		}
	}
}

func TestCoordinatorMapsInterfaceLocalFailure(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	collectors.network.value.interfaces[0].state = unavailable[string](reasonTemporarilyUnavailable, errors.New("gone"))
	collectors.network.value.interfaces[0].rxBytes = unavailable[uint64](reasonPermissionDenied, errors.New("denied"))
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, snapshot.Encode)

	encoded, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	issues := decodeDocument(t, encoded)["issues"].([]any)
	if got := issues[5].(map[string]any)["path"]; got != "/network/interfaces/0/state" {
		t.Errorf("first interface issue path = %v", got)
	}
	if got := issues[6].(map[string]any)["path"]; got != "/network/interfaces/0/rx_bytes" {
		t.Errorf("second interface issue path = %v", got)
	}
	if got := issues[len(issues)-1].(map[string]any)["path"]; got != "/cpu/utilization_percent" {
		t.Errorf("last issue path = %v", got)
	}
}

func TestHostnameFailureDoesNotHideLaterDefect(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	collectors.host.hostname = unavailable[string](reasonTemporarilyUnavailable, errors.New("hostname unavailable"))
	collectors.network = defect[networkObservation](errors.New("network invariant"))
	fatal := make(chan error, 1)
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(err error) { fatal <- err }, time.Now, context.WithTimeout, snapshot.Encode)

	_, err := coordinator.Snapshot(context.Background())
	if !isInternalDefect(err) {
		t.Fatalf("Snapshot() error = %v; want internal defect", err)
	}
	select {
	case <-fatal:
	case <-time.After(time.Second):
		t.Fatal("fatal notification was not delivered")
	}
	wantCalls := []string{"host", "cpu_load", "memory", "root", "network"}
	if got := collectors.recordedCalls(); !equalStrings(got, wantCalls) {
		t.Errorf("collector calls = %v; want %v", got, wantCalls)
	}
}

func TestExpectedHostnameFailureReturnsUnavailableAfterFullCycle(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	collectors.host.hostname = unavailable[string](reasonUnsupported, errors.New("missing"))
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, snapshot.Encode)

	_, err := coordinator.Snapshot(context.Background())
	if !errors.Is(err, snapshot.ErrNoUsefulSnapshot) {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if len(collectors.recordedCalls()) != 5 {
		t.Fatalf("collector calls = %v", collectors.recordedCalls())
	}
}

func TestCoordinatorRejectsZeroUsefulLeaves(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	cause := errors.New("unavailable")
	collectors.host.uptime = unavailable[uint64](reasonTemporarilyUnavailable, cause)
	collectors.cpuLoad.logicalCPUCount = unavailable[uint64](reasonTemporarilyUnavailable, cause)
	collectors.cpuLoad.load = unavailable[loadObservation](reasonTemporarilyUnavailable, cause)
	collectors.memory = unavailable[memoryObservation](reasonTemporarilyUnavailable, cause)
	collectors.root = unavailable[filesystemObservation](reasonTemporarilyUnavailable, cause)
	collectors.network = present(networkObservation{interfaces: []networkInterfaceObservation{}})
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, snapshot.Encode)

	_, err := coordinator.Snapshot(context.Background())
	if !errors.Is(err, snapshot.ErrNoUsefulSnapshot) {
		t.Fatalf("Snapshot() error = %v", err)
	}
}

func TestJoinedRequestsShareOneCycleAndEncoding(t *testing.T) {
	collectors := completeCollectorStub()
	entered := make(chan struct{})
	release := make(chan struct{})
	collectors.hostFn = func(context.Context) hostObservation {
		close(entered)
		<-release
		return collectors.host
	}
	var encodeCalls atomic.Int32
	encode := func(input snapshot.Snapshot) (snapshot.Encoded, error) {
		encodeCalls.Add(1)
		return snapshot.Encode(input)
	}
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, encode)

	const requestCount = 8
	results := make(chan snapshot.Encoded, requestCount)
	errorsResult := make(chan error, requestCount)
	contexts := make([]*observedContext, requestCount)
	for index := range requestCount {
		contexts[index] = newObservedContext(context.Background())
		go func(ctx context.Context) {
			encoded, err := coordinator.Snapshot(ctx)
			results <- encoded
			errorsResult <- err
		}(contexts[index])
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("collection did not start")
	}
	for _, ctx := range contexts {
		select {
		case <-ctx.doneObserved:
		case <-time.After(time.Second):
			t.Fatal("request did not join the active cycle")
		}
	}
	close(release)

	var first []byte
	for range requestCount {
		encoded := <-results
		if err := <-errorsResult; err != nil {
			t.Fatal(err)
		}
		body := encodedBytes(t, encoded)
		if first == nil {
			first = body
		} else if !bytes.Equal(body, first) {
			t.Error("joined request received different encoded bytes")
		}
	}
	if encodeCalls.Load() != 1 {
		t.Errorf("encode calls = %d; want 1", encodeCalls.Load())
	}
	if len(collectors.recordedCalls()) != 5 {
		t.Errorf("collector calls = %v", collectors.recordedCalls())
	}
}

func TestRequesterCancellationDoesNotCancelSharedCycle(t *testing.T) {
	collectors := completeCollectorStub()
	entered := make(chan struct{})
	release := make(chan struct{})
	collectors.hostFn = func(context.Context) hostObservation {
		close(entered)
		<-release
		return collectors.host
	}
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, snapshot.Encode)
	request, cancel := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, err := coordinator.Snapshot(request)
		firstResult <- err
	}()
	<-entered
	cancel()
	if err := <-firstResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled requester error = %v", err)
	}

	joinedContext := newObservedContext(context.Background())
	joinedResult := make(chan error, 1)
	go func() {
		_, err := coordinator.Snapshot(joinedContext)
		joinedResult <- err
	}()
	<-joinedContext.doneObserved
	close(release)
	if err := <-joinedResult; err != nil {
		t.Fatal(err)
	}
	if len(collectors.recordedCalls()) != 5 {
		t.Errorf("collector calls = %v", collectors.recordedCalls())
	}
}

func TestAlreadyCanceledRequesterDoesNotStartCycle(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, snapshot.Encode)
	request, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := coordinator.Snapshot(request)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if calls := collectors.recordedCalls(); len(calls) != 0 {
		t.Errorf("collector calls = %v; want none", calls)
	}
}

func TestDomainTimeoutDiscardsResultAndCycleCanContinue(t *testing.T) {
	collectors := completeCollectorStub()
	memoryEntered := make(chan struct{})
	collectors.memFn = func(ctx context.Context) outcome[memoryObservation] {
		close(memoryEntered)
		<-ctx.Done()
		return collectors.memory
	}
	domainCancel := make(chan context.CancelFunc, 1)
	var timeoutCalls atomic.Int32
	withTimeout := func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(parent)
		if timeoutCalls.Add(1) == 4 {
			domainCancel <- cancel
		}
		return ctx, cancel
	}
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, withTimeout, snapshot.Encode)
	result := make(chan snapshot.Encoded, 1)
	resultErr := make(chan error, 1)
	go func() {
		encoded, err := coordinator.Snapshot(context.Background())
		result <- encoded
		resultErr <- err
	}()
	<-memoryEntered
	(<-domainCancel)()
	encoded := <-result
	if err := <-resultErr; err != nil {
		t.Fatal(err)
	}
	issues := decodeDocument(t, encoded)["issues"].([]any)
	if got := issues[0].(map[string]any)["path"]; got != "/memory" {
		t.Errorf("first issue path = %v; want /memory", got)
	}
}

func TestOrphanedCycleEndsAtControlledCycleDeadline(t *testing.T) {
	collectors := completeCollectorStub()
	cycleCancel := make(chan context.CancelFunc, 2)
	hostEntered := make(chan struct{}, 2)
	collectors.hostFn = func(ctx context.Context) hostObservation {
		hostEntered <- struct{}{}
		<-ctx.Done()
		return collectors.host
	}
	withTimeout := func(parent context.Context, duration time.Duration) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(parent)
		if duration == collectionCycleTimeout {
			cycleCancel <- cancel
		}
		return ctx, cancel
	}
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, withTimeout, snapshot.Encode)
	request, cancelRequest := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, err := coordinator.Snapshot(request)
		first <- err
	}()
	<-hostEntered
	coordinator.mu.Lock()
	orphanedCycle := coordinator.active
	coordinator.mu.Unlock()
	cancelRequest()
	if err := <-first; !errors.Is(err, context.Canceled) {
		t.Fatalf("orphaning request error = %v", err)
	}
	(<-cycleCancel)()
	<-orphanedCycle.done

	second := make(chan error, 1)
	go func() {
		_, err := coordinator.Snapshot(context.Background())
		second <- err
	}()
	<-hostEntered
	(<-cycleCancel)()
	if err := <-second; !errors.Is(err, snapshot.ErrNoUsefulSnapshot) {
		t.Fatalf("fresh cycle error = %v", err)
	}
	if got := collectors.recordedCalls(); len(got) != 2 || got[0] != "host" || got[1] != "host" {
		t.Errorf("collector calls = %v; want two fresh host cycles", got)
	}
}

func TestDefectsAndPanicsFailCoordinatorPermanently(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		prepare func(*collectorStub)
		encode  snapshotEncoder
	}{
		{name: "explicit defect", prepare: func(c *collectorStub) { c.memory = defect[memoryObservation](errors.New("defect")) }, encode: snapshot.Encode},
		{name: "malformed outcome", prepare: func(c *collectorStub) { c.memory = outcome[memoryObservation]{} }, encode: snapshot.Encode},
		{name: "panic", prepare: func(c *collectorStub) { c.hostFn = func(context.Context) hostObservation { panic("boom") } }, encode: snapshot.Encode},
		{name: "encoder defect", prepare: func(*collectorStub) {}, encode: func(snapshot.Snapshot) (snapshot.Encoded, error) {
			return snapshot.Encoded{}, errors.New("encode failed")
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			collectors := completeCollectorStub()
			test.prepare(collectors)
			var fatalCalls atomic.Int32
			coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) { fatalCalls.Add(1) }, time.Now, context.WithTimeout, test.encode)
			_, firstErr := coordinator.Snapshot(context.Background())
			_, secondErr := coordinator.Snapshot(context.Background())
			if !isInternalDefect(firstErr) || secondErr != firstErr {
				t.Fatalf("errors = %v, %v; want same internal defect", firstErr, secondErr)
			}
			if fatalCalls.Load() != 1 {
				t.Errorf("fatal calls = %d; want 1", fatalCalls.Load())
			}
		})
	}
}

func TestNextRequestStartsFreshCycle(t *testing.T) {
	t.Parallel()
	collectors := completeCollectorStub()
	var encodeCalls atomic.Int32
	coordinator := mustTestCoordinator(t, context.Background(), collectors, func(error) {}, time.Now, context.WithTimeout, func(input snapshot.Snapshot) (snapshot.Encoded, error) {
		encodeCalls.Add(1)
		return snapshot.Encode(input)
	})
	first, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	collectors.host.uptime = present(uint64(999))
	second, err := coordinator.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(encodedBytes(t, first), encodedBytes(t, second)) {
		t.Error("fresh cycle reused the prior encoded result")
	}
	if encodeCalls.Load() != 2 || len(collectors.recordedCalls()) != 10 {
		t.Errorf("encode calls = %d, collector calls = %v", encodeCalls.Load(), collectors.recordedCalls())
	}
}

func mustTestCoordinator(t *testing.T, lifecycle context.Context, collectors cycleCollectors, fatal func(error), now func() time.Time, withTimeout timeoutFactory, encode snapshotEncoder) *Coordinator {
	t.Helper()
	coordinator, err := newCoordinator(lifecycle, fatal, coordinatorDependencies{
		collectors: collectors, now: now, withTimeout: withTimeout, encode: encode,
	})
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func encodedBytes(t *testing.T, encoded snapshot.Encoded) []byte {
	t.Helper()
	var body bytes.Buffer
	if _, err := encoded.WriteTo(&body); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func decodeDocument(t *testing.T, encoded snapshot.Encoded) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(encodedBytes(t, encoded)))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	return document
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

type observedContext struct {
	context.Context
	doneObserved chan struct{}
	once         sync.Once
}

func newObservedContext(parent context.Context) *observedContext {
	return &observedContext{Context: parent, doneObserved: make(chan struct{})}
}

func (ctx *observedContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.doneObserved) })
	return ctx.Context.Done()
}
