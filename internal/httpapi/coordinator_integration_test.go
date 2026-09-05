package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Adrien-hue/joy-pi-health/internal/observe"
	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

func TestCoordinatorHTTPIntegration(t *testing.T) {
	t.Parallel()
	provider, err := observe.NewCoordinator(context.Background(), integrationSource{}, func(err error) {
		t.Errorf("unexpected fatal notification: %v", err)
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := mustHandler(t, provider)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, snapshotPath, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if document["schema_version"] != "1.0" || document["uptime_seconds"] != float64(123) {
		t.Errorf("snapshot = %#v", document)
	}
	if got := len(document["issues"].([]any)); got != 6 {
		t.Errorf("issue count = %d; want 6", got)
	}
}

func TestCoordinatorDefectReachesHTTPFatalPath(t *testing.T) {
	t.Parallel()
	var fatalCalls atomic.Int32
	var fatalOnce sync.Once
	notifyFatal := func(error) { fatalOnce.Do(func() { fatalCalls.Add(1) }) }
	provider, err := observe.NewCoordinator(context.Background(), integrationSource{panicHostname: true}, notifyFatal)
	if err != nil {
		t.Fatal(err)
	}
	handler := mustHandlerWithFatal(t, provider, notifyFatal)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, snapshotPath, nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if fatalCalls.Load() != 1 {
		t.Errorf("fatal calls = %d; want 1", fatalCalls.Load())
	}
}

type integrationSource struct {
	panicHostname bool
}

func (source integrationSource) Hostname() (string, error) {
	if source.panicHostname {
		panic("hostname defect")
	}
	return "raspberrypi", nil
}

func (integrationSource) Uptime() ([]byte, error)    { return []byte("123.45 0.0"), nil }
func (integrationSource) CPUOnline() ([]byte, error) { return []byte("0-3"), nil }
func (integrationSource) LoadAverage() ([]byte, error) {
	return []byte("0.1 0.2 0.3 1/1 1"), nil
}
func (integrationSource) MemoryInfo() ([]byte, error) {
	return []byte("MemTotal: 1024 kB\nMemAvailable: 256 kB\n"), nil
}
func (integrationSource) RootFilesystem() (platform.Filesystem, error) {
	return platform.Filesystem{BlockSize: 4096, TotalBlocks: 100, AvailableBlocks: 25}, nil
}
func (integrationSource) Links(context.Context) ([]platform.Link, error) {
	state := uint8(6)
	rx, tx := uint64(10), uint64(20)
	return []platform.Link{{
		Index: 1, Name: "eth0", HardwareType: 1, DevicePath: "/sys/devices/platform/ethernet/net/eth0",
		OperState: &state, RXBytes: &rx, TXBytes: &tx,
	}}, nil
}
