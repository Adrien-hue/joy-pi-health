package observe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

const (
	cpuSamplePeriod       = time.Second
	minimumSampleInterval = 900 * time.Millisecond
	maximumSampleInterval = 1100 * time.Millisecond
	maximumPublicationAge = 1250 * time.Millisecond
	maximumCPUStatInput   = 512
	maximumBootIDInput    = 128
)

type cpuCounters [8]uint64

type cpuSample struct {
	counters cpuCounters
	topology cpuTopology
	bootID   string
	uptime   uint64
}

type timedCPUSample struct {
	value cpuSample
	at    time.Duration
}

type cpuPublication struct {
	utilizationPercent float64
	sampledAt          time.Duration
	topology           cpuTopology
}

type cpuPublicationProvider interface {
	latestCPU() (outcome[cpuPublication], time.Duration)
}

type cpuObserverDependencies struct {
	now  func() time.Duration
	wait func(context.Context, time.Duration) error
}

// CPUObserver owns the one-hertz trailing utilization observation.
type CPUObserver struct {
	ctx    context.Context
	cancel context.CancelFunc
	source platform.CPUSource
	fatal  func(error)
	now    func() time.Duration
	wait   func(context.Context, time.Duration) error

	publication atomic.Pointer[outcome[cpuPublication]]

	mu       sync.Mutex
	baseline *timedCPUSample
	started  bool
	stopped  bool
	done     chan struct{}
}

// NewCPUObserver constructs the production observer. Start performs its
// immediate baseline attempt and begins the single scheduled goroutine.
func NewCPUObserver(source platform.CPUSource, fatal func(error)) (*CPUObserver, error) {
	if source == nil || fatal == nil {
		return nil, errors.New("CPU observer dependencies are required")
	}
	origin := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	observer := &CPUObserver{
		ctx: ctx, cancel: cancel, source: source, fatal: fatal,
		now: func() time.Duration { return time.Since(origin) }, done: make(chan struct{}),
	}
	observer.wait = func(ctx context.Context, target time.Duration) error {
		delay := target - observer.now()
		if delay <= 0 {
			return nil
		}
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
	observer.store(unavailable[cpuPublication](reasonTemporarilyUnavailable, errors.New("CPU observer has not established a baseline")))
	return observer, nil
}

func newCPUObserver(source platform.CPUSource, fatal func(error), dependencies cpuObserverDependencies) (*CPUObserver, error) {
	if source == nil || fatal == nil || dependencies.now == nil || dependencies.wait == nil {
		return nil, errors.New("CPU observer dependencies are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	observer := &CPUObserver{
		ctx: ctx, cancel: cancel, source: source, fatal: fatal,
		now: dependencies.now, wait: dependencies.wait, done: make(chan struct{}),
	}
	observer.store(unavailable[cpuPublication](reasonTemporarilyUnavailable, errors.New("CPU observer has not established a baseline")))
	return observer, nil
}

// Start makes one immediate baseline attempt and then starts scheduled work.
func (observer *CPUObserver) Start() (err error) {
	observer.mu.Lock()
	if observer.started || observer.stopped {
		observer.mu.Unlock()
		return errors.New("CPU observer cannot be started in its current state")
	}
	observer.started = true
	observer.mu.Unlock()

	defer func() {
		if recovered := recover(); recovered != nil {
			err = internalDefect(fmt.Errorf("CPU observer startup panic: %v", recovered))
			observer.publishDefect(err)
			observer.fatal(err)
			observer.cancel()
			close(observer.done)
		}
	}()
	initialAt := observer.now()
	observer.observe(initialAt)
	go observer.run(initialAt + cpuSamplePeriod)
	return nil
}

func (observer *CPUObserver) run(next time.Duration) {
	defer close(observer.done)
	var fatalErr error
	defer func() {
		if recovered := recover(); recovered != nil {
			fatalErr = internalDefect(fmt.Errorf("CPU observer panic: %v", recovered))
		}
		if fatalErr == nil && observer.ctx.Err() == nil {
			fatalErr = internalDefect(errors.New("CPU observer stopped unexpectedly"))
		}
		if fatalErr != nil {
			observer.publishDefect(fatalErr)
			observer.fatal(fatalErr)
		}
	}()

	for {
		if err := observer.wait(observer.ctx, next); err != nil {
			if observer.ctx.Err() != nil {
				return
			}
			fatalErr = internalDefect(fmt.Errorf("CPU observer scheduler: %w", err))
			return
		}
		if observer.ctx.Err() != nil {
			return
		}
		observer.observe(observer.now())
		now := observer.now()
		for next <= now {
			next += cpuSamplePeriod
		}
	}
}

// Close cancels scheduled work and joins the observer within ctx.
func (observer *CPUObserver) Close(ctx context.Context) error {
	observer.mu.Lock()
	observer.stopped = true
	started := observer.started
	observer.cancel()
	observer.mu.Unlock()
	if !started {
		return nil
	}
	select {
	case <-observer.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (observer *CPUObserver) latestCPU() (outcome[cpuPublication], time.Duration) {
	value := observer.publication.Load()
	if value == nil {
		return defect[cpuPublication](errors.New("CPU publication is absent")), observer.now()
	}
	return *value, observer.now()
}

func (observer *CPUObserver) observe(at time.Duration) {
	sample := collectCPUSample(observer.source)
	if err := validateOutcome(sample); err != nil {
		panic(err)
	}

	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.stopped || observer.ctx.Err() != nil {
		return
	}
	if sample.state != statePresent {
		observer.baseline = nil
		observer.store(unavailable[cpuPublication](sample.reason, sample.cause))
		return
	}

	current := timedCPUSample{value: sample.value, at: at}
	if observer.baseline == nil {
		observer.baseline = &current
		observer.store(unavailable[cpuPublication](reasonTemporarilyUnavailable, errors.New("CPU observer requires a second valid sample")))
		return
	}
	previous := *observer.baseline
	observer.baseline = &current
	utilization, err := calculateCPUUtilization(previous, current)
	if err != nil {
		observer.store(unavailable[cpuPublication](reasonTemporarilyUnavailable, err))
		return
	}
	observer.store(present(cpuPublication{utilizationPercent: utilization, sampledAt: at, topology: current.value.topology}))
}

func (observer *CPUObserver) store(value outcome[cpuPublication]) {
	copy := value
	observer.publication.Store(&copy)
}

func (observer *CPUObserver) publishDefect(err error) {
	observer.mu.Lock()
	defer observer.mu.Unlock()
	if observer.stopped || observer.ctx.Err() != nil {
		return
	}
	observer.store(defect[cpuPublication](err))
}

func collectCPUSample(source platform.CPUSource) outcome[cpuSample] {
	statData, err := source.CPUStat()
	if err != nil {
		return acquisitionFailure[cpuSample](err, reasonUnsupported)
	}
	counters, err := parseCPUCounters(statData)
	if err != nil {
		return unavailable[cpuSample](reasonTemporarilyUnavailable, err)
	}
	topologyData, err := source.CPUOnline()
	if err != nil {
		return acquisitionFailure[cpuSample](err, reasonUnsupported)
	}
	topology, err := parseCPUTopology(topologyData)
	if err != nil {
		return unavailable[cpuSample](reasonTemporarilyUnavailable, err)
	}
	bootData, err := source.BootID()
	if err != nil {
		return acquisitionFailure[cpuSample](err, reasonUnsupported)
	}
	bootID, err := parseBootID(bootData)
	if err != nil {
		return unavailable[cpuSample](reasonTemporarilyUnavailable, err)
	}
	uptimeData, err := source.Uptime()
	if err != nil {
		return acquisitionFailure[cpuSample](err, reasonUnsupported)
	}
	uptime, err := parseUptime(uptimeData)
	if err != nil {
		return unavailable[cpuSample](reasonTemporarilyUnavailable, err)
	}
	return present(cpuSample{counters: counters, topology: topology, bootID: bootID, uptime: uptime})
}

func parseCPUCounters(data []byte) (cpuCounters, error) {
	if len(data) == 0 || len(data) > maximumCPUStatInput {
		return cpuCounters{}, errors.New("CPU counter input is empty or oversized")
	}
	line, _, _ := bytes.Cut(data, []byte{'\n'})
	fields := bytes.Fields(line)
	if len(fields) < 9 || string(fields[0]) != "cpu" {
		return cpuCounters{}, errors.New("aggregate CPU counters are missing")
	}
	var counters cpuCounters
	for index := range counters {
		value, err := strconv.ParseUint(string(fields[index+1]), 10, 64)
		if err != nil {
			return cpuCounters{}, errors.New("aggregate CPU counters contain an invalid value")
		}
		counters[index] = value
	}
	return counters, nil
}

func parseBootID(data []byte) (string, error) {
	if len(data) == 0 || len(data) > maximumBootIDInput {
		return "", errors.New("boot ID is empty or oversized")
	}
	value := strings.TrimSpace(string(data))
	if len(value) != 36 {
		return "", errors.New("boot ID is not a UUID")
	}
	for index, character := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return "", errors.New("boot ID is not a UUID")
			}
			continue
		}
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') || (character >= 'A' && character <= 'F')) {
			return "", errors.New("boot ID is not a UUID")
		}
	}
	return strings.ToLower(value), nil
}

func calculateCPUUtilization(previous, current timedCPUSample) (float64, error) {
	interval := current.at - previous.at
	if interval < minimumSampleInterval || interval > maximumSampleInterval {
		return 0, errors.New("CPU sample interval is outside the valid range")
	}
	if previous.value.bootID != current.value.bootID || current.value.uptime < previous.value.uptime {
		return 0, errors.New("CPU counter epoch changed")
	}
	if previous.value.topology != current.value.topology {
		return 0, errors.New("CPU topology changed")
	}

	var deltas cpuCounters
	for index := range deltas {
		if current.value.counters[index] < previous.value.counters[index] {
			return 0, errors.New("CPU counter decreased")
		}
		deltas[index] = current.value.counters[index] - previous.value.counters[index]
	}
	busy, err := sumUint64(deltas[0], deltas[1], deltas[2], deltas[5], deltas[6])
	if err != nil {
		return 0, err
	}
	total, err := sumUint64(busy, deltas[3], deltas[4], deltas[7])
	if err != nil || total == 0 || busy > total {
		return 0, errors.New("CPU counter delta is inconsistent")
	}
	utilization := 100 * float64(busy) / float64(total)
	if math.IsNaN(utilization) || math.IsInf(utilization, 0) || utilization < 0 || utilization > 100 {
		return 0, errors.New("CPU utilization is invalid")
	}
	return utilization, nil
}

func sumUint64(values ...uint64) (uint64, error) {
	var total uint64
	for _, value := range values {
		if total > math.MaxUint64-value {
			return 0, errors.New("CPU counter delta overflows")
		}
		total += value
	}
	return total, nil
}
