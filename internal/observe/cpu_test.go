package observe

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	"time"
)

const testBootID = "12345678-1234-1234-1234-123456789abc"

type cpuSourceStub struct {
	mu        sync.Mutex
	stat      []byte
	statErr   error
	topology  []byte
	topErr    error
	bootID    []byte
	bootErr   error
	uptime    []byte
	uptimeErr error
	panicStat bool
	blockStat <-chan struct{}
}

func standardCPUSource() *cpuSourceStub {
	return &cpuSourceStub{
		stat: []byte("cpu 10 0 10 80 0 0 0 0 0 0\n"), topology: []byte("0-3\n"),
		bootID: []byte(testBootID + "\n"), uptime: []byte("100.00 0.00\n"),
	}
}

func (source *cpuSourceStub) CPUStat() ([]byte, error) {
	if source.blockStat != nil {
		<-source.blockStat
	}
	if source.panicStat {
		panic("CPU source panic")
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	return append([]byte(nil), source.stat...), source.statErr
}

func (source *cpuSourceStub) CPUOnline() ([]byte, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	return append([]byte(nil), source.topology...), source.topErr
}

func (source *cpuSourceStub) BootID() ([]byte, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	return append([]byte(nil), source.bootID...), source.bootErr
}

func (source *cpuSourceStub) Uptime() ([]byte, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	return append([]byte(nil), source.uptime...), source.uptimeErr
}

func (source *cpuSourceStub) set(stat, topology, bootID, uptime string) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.stat = []byte(stat)
	source.topology = []byte(topology)
	source.bootID = []byte(bootID)
	source.uptime = []byte(uptime)
	source.statErr = nil
}

func TestParseCPUCountersAndBootID(t *testing.T) {
	t.Parallel()
	counters, err := parseCPUCounters([]byte("cpu 1 2 3 4 5 6 7 8 90 91\ncpu0 1 2 3 4 5 6 7 8\n"))
	if err != nil || counters != (cpuCounters{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Fatalf("parseCPUCounters() = %#v, %v", counters, err)
	}
	for _, input := range []string{"", "cpu0 1 2 3 4 5 6 7 8", "cpu 1 2 3", "cpu 1 2 x 4 5 6 7 8"} {
		if _, err := parseCPUCounters([]byte(input)); err == nil {
			t.Errorf("parseCPUCounters(%q) succeeded", input)
		}
	}
	if got, err := parseBootID([]byte("12345678-1234-ABCD-1234-123456789ABC\n")); err != nil || got != testBootID[:14]+"abcd"+testBootID[18:] {
		t.Fatalf("parseBootID() = %q, %v", got, err)
	}
	for _, input := range []string{"", "not-a-uuid", "123456781234-1234-1234-123456789abc", "12345678-1234-1234-1234-123456789abz"} {
		if _, err := parseBootID([]byte(input)); err == nil {
			t.Errorf("parseBootID(%q) succeeded", input)
		}
	}
}

func TestCalculateCPUUtilization(t *testing.T) {
	t.Parallel()
	base := sampleAt(cpuCounters{10, 0, 10, 80, 0, 0, 0, 0}, time.Second)
	tests := []struct {
		name string
		at   time.Duration
		cpu  cpuCounters
		want float64
		ok   bool
	}{
		{name: "inclusive short boundary", at: 1900 * time.Millisecond, cpu: cpuCounters{10, 0, 10, 180, 0, 0, 0, 0}, want: 0, ok: true},
		{name: "inclusive long boundary", at: 2100 * time.Millisecond, cpu: cpuCounters{110, 0, 10, 80, 0, 0, 0, 0}, want: 100, ok: true},
		{name: "mixed excludes iowait and steal", at: 2 * time.Second, cpu: cpuCounters{35, 0, 35, 115, 15, 5, 5, 10}, want: 50, ok: true},
		{name: "too short", at: 1899 * time.Millisecond, cpu: cpuCounters{20, 0, 10, 90, 0, 0, 0, 0}, ok: false},
		{name: "too long", at: 2101 * time.Millisecond, cpu: cpuCounters{20, 0, 10, 90, 0, 0, 0, 0}, ok: false},
		{name: "counter decrease", at: 2 * time.Second, cpu: cpuCounters{9, 0, 10, 90, 0, 0, 0, 0}, ok: false},
		{name: "zero delta", at: 2 * time.Second, cpu: base.value.counters, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := sampleAt(test.cpu, test.at)
			got, err := calculateCPUUtilization(base, current)
			if (err == nil) != test.ok || test.ok && math.Abs(got-test.want) > 0.000001 {
				t.Fatalf("calculateCPUUtilization() = %v, %v; want %v, success %t", got, err, test.want, test.ok)
			}
		})
	}

	reboot := sampleAt(cpuCounters{20, 0, 20, 160, 0, 0, 0, 0}, 2*time.Second)
	reboot.value.bootID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	if _, err := calculateCPUUtilization(base, reboot); err == nil {
		t.Error("boot ID change was accepted")
	}
	uptimeRegression := sampleAt(cpuCounters{20, 0, 20, 160, 0, 0, 0, 0}, 2*time.Second)
	uptimeRegression.value.uptime = 99
	if _, err := calculateCPUUtilization(base, uptimeRegression); err == nil {
		t.Error("uptime regression was accepted")
	}
	topologyChange := sampleAt(cpuCounters{20, 0, 20, 160, 0, 0, 0, 0}, 2*time.Second)
	topologyChange.value.topology = cpuTopology{count: 2, signature: "0-1"}
	if _, err := calculateCPUUtilization(base, topologyChange); err == nil {
		t.Error("topology change was accepted")
	}
	overflow := sampleAt(cpuCounters{math.MaxUint64, math.MaxUint64, 10, 80, 0, 0, 0, 0}, 2*time.Second)
	if _, err := calculateCPUUtilization(base, overflow); err == nil {
		t.Error("overflowing delta was accepted")
	}
}

func sampleAt(counters cpuCounters, at time.Duration) timedCPUSample {
	return timedCPUSample{value: cpuSample{
		counters: counters, topology: cpuTopology{count: 4, signature: "0-3"}, bootID: testBootID, uptime: 100,
	}, at: at}
}

func TestCPUObserverBaselinePublicationAndRecovery(t *testing.T) {
	t.Parallel()
	source := standardCPUSource()
	now := time.Duration(0)
	observer := mustCPUObserver(t, source, func() time.Duration { return now })

	observer.observe(now)
	assertCPUUnavailable(t, observer)
	source.set("cpu 20 0 20 160 0 0 0 0\n", "0-3\n", testBootID, "101.00 0")
	now = time.Second
	observer.observe(now)
	publication, _ := observer.latestCPU()
	if publication.state != statePresent || math.Abs(publication.value.utilizationPercent-20) > 0.000001 {
		t.Fatalf("publication = %#v; want 20 percent", publication)
	}

	source.statErr = errors.New("temporary read failure")
	now = 2 * time.Second
	observer.observe(now)
	assertCPUUnavailable(t, observer)
	source.set("cpu 30 0 30 240 0 0 0 0\n", "0-3", testBootID, "103.00 0")
	now = 3 * time.Second
	observer.observe(now)
	assertCPUUnavailable(t, observer)
	source.set("cpu 40 0 40 320 0 0 0 0\n", "0-3", testBootID, "104.00 0")
	now = 4 * time.Second
	observer.observe(now)
	if publication, _ = observer.latestCPU(); publication.state != statePresent {
		t.Fatalf("publication after recovery = %#v", publication)
	}
}

func TestCPUObserverDelayedSampleRebaselines(t *testing.T) {
	t.Parallel()
	source := standardCPUSource()
	now := time.Duration(0)
	observer := mustCPUObserver(t, source, func() time.Duration { return now })
	observer.observe(now)
	source.set("cpu 20 0 20 160 0 0 0 0", "0-3", testBootID, "102.00 0")
	now = 2 * time.Second
	observer.observe(now)
	assertCPUUnavailable(t, observer)
	source.set("cpu 30 0 30 240 0 0 0 0", "0-3", testBootID, "103.00 0")
	now = 3 * time.Second
	observer.observe(now)
	if publication, _ := observer.latestCPU(); publication.state != statePresent {
		t.Fatalf("publication after delayed rebaseline = %#v", publication)
	}
}

func TestCPUObserverEpochResetAndTopologyChangesRebaseline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		changed   [4]string
		recovered [4]string
	}{
		{
			name:      "counter reset",
			changed:   [4]string{"cpu 1 0 1 8 0 0 0 0", "0-3", testBootID, "101.00 0"},
			recovered: [4]string{"cpu 2 0 2 16 0 0 0 0", "0-3", testBootID, "102.00 0"},
		},
		{
			name:      "reboot epoch",
			changed:   [4]string{"cpu 1 0 1 8 0 0 0 0", "0-3", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "1.00 0"},
			recovered: [4]string{"cpu 2 0 2 16 0 0 0 0", "0-3", "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "2.00 0"},
		},
		{
			name:      "logical topology",
			changed:   [4]string{"cpu 20 0 20 160 0 0 0 0", "0-1", testBootID, "101.00 0"},
			recovered: [4]string{"cpu 30 0 30 240 0 0 0 0", "0-1", testBootID, "102.00 0"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := standardCPUSource()
			now := time.Duration(0)
			observer := mustCPUObserver(t, source, func() time.Duration { return now })
			observer.observe(now)
			source.set(test.changed[0], test.changed[1], test.changed[2], test.changed[3])
			now = time.Second
			observer.observe(now)
			assertCPUUnavailable(t, observer)
			source.set(test.recovered[0], test.recovered[1], test.recovered[2], test.recovered[3])
			now = 2 * time.Second
			observer.observe(now)
			if publication, _ := observer.latestCPU(); publication.state != statePresent {
				t.Fatalf("publication after rebaseline = %#v", publication)
			}
		})
	}
}

func TestCPUObserverScheduledLifecycleAndFatalPanic(t *testing.T) {
	t.Parallel()
	source := standardCPUSource()
	var now time.Duration
	targets := make(chan time.Duration, 2)
	advance := make(chan struct{}, 2)
	fatal := make(chan error, 1)
	observer, err := newCPUObserver(source, func(err error) { fatal <- err }, cpuObserverDependencies{
		now: func() time.Duration { return now },
		wait: func(ctx context.Context, target time.Duration) error {
			targets <- target
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-advance:
				return nil
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.Start(); err != nil {
		t.Fatalf("observer start = %v", err)
	}
	if target := waitCPUResult(t, targets); target != time.Second {
		t.Fatalf("first target = %v", target)
	}
	source.set("cpu 20 0 20 160 0 0 0 0", "0-3", testBootID, "101.00 0")
	now = time.Second
	advance <- struct{}{}
	if target := waitCPUResult(t, targets); target != 2*time.Second {
		t.Fatalf("second target = %v", target)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := observer.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-fatal:
		t.Fatalf("clean shutdown notified fatal: %v", err)
	default:
	}

	panicSource := standardCPUSource()
	panicSource.panicStat = true
	panicFatal := make(chan error, 1)
	panicObserver := mustCPUObserverWithFatal(t, panicSource, func() time.Duration { return 0 }, func(err error) { panicFatal <- err })
	if err := panicObserver.Start(); !isInternalDefect(err) {
		t.Fatalf("Start() error = %v; want internal defect", err)
	}
	if err := waitCPUResult(t, panicFatal); !isInternalDefect(err) {
		t.Fatalf("fatal error = %v; want internal defect", err)
	}
}

func TestCPUObserverSchedulerFailureIsFatal(t *testing.T) {
	t.Parallel()
	fatal := make(chan error, 1)
	observer, err := newCPUObserver(standardCPUSource(), func(err error) { fatal <- err }, cpuObserverDependencies{
		now:  func() time.Duration { return 0 },
		wait: func(context.Context, time.Duration) error { return errors.New("scheduler stopped") },
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := observer.Start(); err != nil {
		t.Fatal(err)
	}
	if err := waitCPUResult(t, fatal); !isInternalDefect(err) {
		t.Fatalf("fatal error = %v; want internal defect", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := observer.Close(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestCPUObserverDoesNotPublishAfterCancellation(t *testing.T) {
	release := make(chan struct{})
	source := standardCPUSource()
	now := time.Duration(0)
	observer := mustCPUObserver(t, source, func() time.Duration { return now })
	observer.observe(now)
	source.set("cpu 20 0 20 160 0 0 0 0", "0-3", testBootID, "101.00 0")
	source.blockStat = release
	now = time.Second
	done := make(chan struct{})
	go func() {
		observer.observe(now)
		close(done)
	}()
	if err := observer.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(release)
	<-done
	assertCPUUnavailable(t, observer)
}

func mustCPUObserver(t *testing.T, source *cpuSourceStub, now func() time.Duration) *CPUObserver {
	t.Helper()
	return mustCPUObserverWithFatal(t, source, now, func(err error) { t.Errorf("unexpected fatal error: %v", err) })
}

func mustCPUObserverWithFatal(t *testing.T, source *cpuSourceStub, now func() time.Duration, fatal func(error)) *CPUObserver {
	t.Helper()
	observer, err := newCPUObserver(source, fatal, cpuObserverDependencies{
		now: now, wait: func(context.Context, time.Duration) error { return errors.New("not used") },
	})
	if err != nil {
		t.Fatal(err)
	}
	return observer
}

func assertCPUUnavailable(t *testing.T, observer *CPUObserver) {
	t.Helper()
	publication, _ := observer.latestCPU()
	if publication.state != stateUnavailable || publication.reason != reasonTemporarilyUnavailable {
		t.Fatalf("CPU publication = %#v; want temporarily unavailable", publication)
	}
}

func waitCPUResult[T any](t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for CPU observer")
		var zero T
		return zero
	}
}
