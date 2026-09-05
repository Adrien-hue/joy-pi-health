//go:build linux

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"testing"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/observe"
	"github.com/Adrien-hue/joy-pi-health/internal/platform"
)

func TestProductionGenericSnapshotEndToEnd(t *testing.T) {
	lifecycle, cancelLifecycle := context.WithCancel(context.Background())
	defer cancelLifecycle()
	fatal := make(chan error, 1)
	notifyFatal := func(err error) {
		select {
		case fatal <- err:
		default:
		}
	}
	provider, err := observe.NewCoordinator(lifecycle, platform.NewSource(), notifyFatal)
	if err != nil {
		t.Fatal(err)
	}
	server, err := Listen(lifecycle, netip.MustParseAddr("127.0.0.1"), 0, provider, notifyFatal, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	serveResult := make(chan error, 1)
	go func() { serveResult <- server.Serve() }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() error = %v", err)
		}
		if err := <-serveResult; err != nil {
			t.Errorf("Serve() error = %v", err)
		}
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + server.address().String() + snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, body = %s", response.StatusCode, body)
	}
	var document map[string]any
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	host := document["host"].(map[string]any)
	if host["hostname"] == "" {
		t.Error("hostname is empty")
	}
	cpu := document["cpu"].(map[string]any)
	if cpu["logical_cpu_count"] == nil || cpu["utilization_percent"] != nil {
		t.Errorf("cpu = %#v", cpu)
	}
	select {
	case err := <-fatal:
		t.Fatalf("unexpected fatal notification: %v", err)
	default:
	}
}
