package observe

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"reflect"
	"testing"

	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

type sourceStub struct {
	hostname   string
	hostErr    error
	uptime     []byte
	uptimeErr  error
	cpuOnline  []byte
	cpuErr     error
	load       []byte
	loadErr    error
	memory     []byte
	memoryErr  error
	filesystem platform.Filesystem
	fsErr      error
	links      []platform.Link
	linksErr   error
}

func (source sourceStub) Hostname() (string, error)  { return source.hostname, source.hostErr }
func (source sourceStub) Uptime() ([]byte, error)    { return source.uptime, source.uptimeErr }
func (source sourceStub) CPUOnline() ([]byte, error) { return source.cpuOnline, source.cpuErr }
func (source sourceStub) LoadAverage() ([]byte, error) {
	return source.load, source.loadErr
}
func (source sourceStub) MemoryInfo() ([]byte, error) { return source.memory, source.memoryErr }
func (source sourceStub) RootFilesystem() (platform.Filesystem, error) {
	return source.filesystem, source.fsErr
}
func (source sourceStub) Links(context.Context) ([]platform.Link, error) {
	return source.links, source.linksErr
}

func TestParseUptime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
		want uint64
		ok   bool
	}{
		{name: "fraction discarded", data: "123.99 4.00\n", want: 123, ok: true},
		{name: "integer", data: "0 0", want: 0, ok: true},
		{name: "negative", data: "-1.0 0", ok: false},
		{name: "missing integer", data: ".5 0", ok: false},
		{name: "bad fraction", data: "1.x 0", ok: false},
		{name: "empty", data: "", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseUptime([]byte(test.data))
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("parseUptime() = %d, %v; want %d, success %t", got, err, test.want, test.ok)
			}
		})
	}
}

func TestParseCPUList(t *testing.T) {
	t.Parallel()
	tests := []struct {
		data string
		want uint64
		ok   bool
	}{
		{data: "0-3\n", want: 4, ok: true},
		{data: "0-1,4,7-8", want: 5, ok: true},
		{data: "1,0", ok: false},
		{data: "0-2,2-3", ok: false},
		{data: "0-", ok: false},
		{data: "", ok: false},
	}
	for _, test := range tests {
		t.Run(test.data, func(t *testing.T) {
			t.Parallel()
			got, err := parseCPUList([]byte(test.data))
			if (err == nil) != test.ok || got != test.want {
				t.Fatalf("parseCPUList() = %d, %v; want %d, success %t", got, err, test.want, test.ok)
			}
		})
	}
}

func TestParseLoad(t *testing.T) {
	t.Parallel()
	got, err := parseLoad([]byte("0.25 1.5 2.75 1/10 42\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := loadObservation{oneMinute: 0.25, fiveMinutes: 1.5, fifteenMinutes: 2.75}
	if got != want {
		t.Fatalf("parseLoad() = %#v; want %#v", got, want)
	}
	for _, input := range []string{"", "1 2", "-1 2 3", "NaN 2 3", "Inf 2 3"} {
		if _, err := parseLoad([]byte(input)); err == nil {
			t.Errorf("parseLoad(%q) succeeded", input)
		}
	}
}

func TestParseMemory(t *testing.T) {
	t.Parallel()
	data := []byte("MemTotal:       1024 kB\nIgnored: 1 kB\nMemAvailable:    256 kB\n")
	got, err := parseMemory(data)
	if err != nil {
		t.Fatal(err)
	}
	want := memoryObservation{totalBytes: 1024 * 1024, availableBytes: 256 * 1024, usedBytes: 768 * 1024}
	if got != want {
		t.Fatalf("parseMemory() = %#v; want %#v", got, want)
	}
	for _, input := range []string{
		"MemTotal: 1 kB\n",
		"MemTotal: 1 kB\nMemTotal: 1 kB\nMemAvailable: 1 kB\n",
		"MemTotal: 1 MB\nMemAvailable: 1 kB\n",
		"MemTotal: 1 kB\nMemAvailable: 2 kB\n",
	} {
		if _, err := parseMemory([]byte(input)); err == nil {
			t.Errorf("parseMemory(%q) succeeded", input)
		}
	}
}

func TestGenericCollectors(t *testing.T) {
	t.Parallel()
	source := sourceStub{
		hostname:   "pi",
		uptime:     []byte("42.9 10.0"),
		cpuOnline:  []byte("0-3"),
		load:       []byte("0.1 0.2 0.3 1/1 1"),
		memory:     []byte("MemTotal: 1024 kB\nMemAvailable: 256 kB\n"),
		filesystem: platform.Filesystem{BlockSize: 4096, TotalBlocks: 100, AvailableBlocks: 25},
	}
	host := collectHost(context.Background(), source)
	if host.hostname.value != "pi" || host.uptime.value != 42 {
		t.Fatalf("unexpected host observation: %#v", host)
	}
	cpu := collectCPULoad(context.Background(), source)
	if cpu.logicalCPUCount.value != 4 || cpu.load.value.oneMinute != 0.1 {
		t.Fatalf("unexpected CPU/load observation: %#v", cpu)
	}
	memory := collectMemory(context.Background(), source)
	if memory.state != statePresent || memory.value.usedBytes != 768*1024 {
		t.Fatalf("unexpected memory observation: %#v", memory)
	}
	root := collectRootFilesystem(context.Background(), source)
	if root.state != statePresent || root.value.usedBytes != 75*4096 {
		t.Fatalf("unexpected filesystem observation: %#v", root)
	}
}

func TestAcquisitionFailureClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err     error
		missing availabilityReason
		state   outcomeState
		reason  availabilityReason
	}{
		{err: fs.ErrNotExist, missing: reasonUnsupported, state: stateUnavailable, reason: reasonUnsupported},
		{err: fs.ErrPermission, missing: reasonUnsupported, state: stateUnavailable, reason: reasonPermissionDenied},
		{err: errors.New("temporary"), missing: reasonUnsupported, state: stateUnavailable, reason: reasonTemporarilyUnavailable},
		{err: nil, missing: reasonUnsupported, state: stateDefect},
	}
	for _, test := range tests {
		got := acquisitionFailure[uint64](test.err, test.missing)
		if got.state != test.state || got.reason != test.reason {
			t.Errorf("acquisitionFailure(%v) = %#v", test.err, got)
		}
	}
}

func TestRootFilesystemRejectsInconsistentValues(t *testing.T) {
	t.Parallel()
	tests := []platform.Filesystem{
		{BlockSize: 0, TotalBlocks: 1},
		{BlockSize: 2, TotalBlocks: math.MaxUint64},
		{BlockSize: 1, TotalBlocks: 1, AvailableBlocks: 2},
	}
	for _, filesystem := range tests {
		got := collectRootFilesystem(context.Background(), sourceStub{filesystem: filesystem})
		if got.state != stateUnavailable || got.reason != reasonTemporarilyUnavailable {
			t.Errorf("collectRootFilesystem(%#v) = %#v", filesystem, got)
		}
	}
}

func TestCancelledCollectionDoesNotReadSource(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := collectMemory(ctx, sourceStub{memory: []byte("MemTotal: 1 kB\nMemAvailable: 1 kB\n")})
	if got.state != stateUnavailable || !errors.Is(got.cause, context.Canceled) {
		t.Fatalf("collectMemory() = %#v", got)
	}
}

func TestOutcomeConstructors(t *testing.T) {
	t.Parallel()
	if got := present(uint64(7)); got.state != statePresent || got.value != 7 {
		t.Fatalf("present() = %#v", got)
	}
	if got := unavailable[uint64](0, errors.New("bad")); got.state != stateDefect {
		t.Fatalf("invalid unavailable() = %#v", got)
	}
	if got := defect[uint64](nil); got.state != stateDefect || got.cause == nil {
		t.Fatalf("defect(nil) = %#v", got)
	}
}

func TestMemoryObservationShape(t *testing.T) {
	t.Parallel()
	got, err := parseMemory([]byte("MemTotal: 4 kB\nMemAvailable: 1 kB\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := memoryObservation{totalBytes: 4096, availableBytes: 1024, usedBytes: 3072}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMemory() = %#v; want %#v", got, want)
	}
}
