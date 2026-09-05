package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

func TestRequestValidation(t *testing.T) {
	encoded := encodedCompleteSnapshot(t)
	providerCalls := atomic.Int32{}
	handler := mustHandler(t, providerFunc(func(context.Context) (snapshot.Encoded, error) {
		providerCalls.Add(1)
		return encoded, nil
	}))

	tests := []struct {
		name       string
		method     string
		target     string
		body       io.Reader
		configure  func(*http.Request)
		wantStatus int
		wantAllow  string
		wantClose  bool
		wantCalls  int32
	}{
		{name: "snapshot", method: http.MethodGet, target: snapshotPath, wantStatus: http.StatusOK, wantCalls: 1},
		{name: "unknown route", method: http.MethodGet, target: "/unknown", wantStatus: http.StatusNotFound},
		{name: "encoded route", method: http.MethodGet, target: "/v1/%73napshot", wantStatus: http.StatusNotFound},
		{name: "unknown route takes precedence", method: http.MethodPost, target: "/unknown", wantStatus: http.StatusNotFound},
		{name: "unsupported method", method: http.MethodPost, target: snapshotPath, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET"},
		{name: "head", method: http.MethodHead, target: snapshotPath, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET"},
		{name: "options", method: http.MethodOptions, target: snapshotPath, wantStatus: http.StatusMethodNotAllowed, wantAllow: "GET"},
		{name: "query", method: http.MethodGet, target: snapshotPath + "?value=1", wantStatus: http.StatusBadRequest},
		{name: "zero content length", method: http.MethodGet, target: snapshotPath, body: bytes.NewReader(nil), wantStatus: http.StatusOK, wantCalls: 1},
		{name: "positive content length", method: http.MethodGet, target: snapshotPath, body: bytes.NewReader([]byte("x")), wantStatus: http.StatusBadRequest, wantClose: true},
		{name: "transfer encoding", method: http.MethodGet, target: snapshotPath, configure: func(r *http.Request) {
			r.TransferEncoding = []string{"chunked"}
			r.ContentLength = -1
		}, wantStatus: http.StatusBadRequest, wantClose: true},
		{name: "unknown content length", method: http.MethodGet, target: snapshotPath, configure: func(r *http.Request) {
			r.ContentLength = -1
		}, wantStatus: http.StatusBadRequest, wantClose: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			providerCalls.Store(0)
			request := httptest.NewRequest(test.method, test.target, test.body)
			request.Header.Set("Accept", "application/xml")
			request.Header.Set("Content-Type", "text/plain")
			if test.configure != nil {
				test.configure(request)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if got := response.Header().Get("Allow"); got != test.wantAllow {
				t.Errorf("Allow = %q, want %q", got, test.wantAllow)
			}
			if got := response.Header().Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", got)
			}
			if got := response.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			if response.Header().Get("ETag") != "" || response.Header().Get("Last-Modified") != "" || response.Header().Get("Content-Encoding") != "" {
				t.Error("response contains a prohibited validator or content encoding")
			}
			if test.wantClose && response.Header().Get("Connection") != "close" {
				t.Error("body rejection did not close the connection")
			}
			if got := providerCalls.Load(); got != test.wantCalls {
				t.Errorf("provider calls = %d, want %d", got, test.wantCalls)
			}
			if test.wantStatus != http.StatusOK {
				want := `{"error":{"code":"invalid_request","message":"The request is invalid."}}`
				if response.Body.String() != want {
					t.Errorf("body = %q, want %q", response.Body.String(), want)
				}
			}
		})
	}
}

func TestRejectedBodyIsNotConsumed(t *testing.T) {
	body := &observedBody{}
	request := httptest.NewRequest(http.MethodGet, snapshotPath, nil)
	request.Body = body
	request.ContentLength = 1
	response := httptest.NewRecorder()
	mustHandler(t, unavailableProvider{}).ServeHTTP(response, request)
	if body.reads != 0 || body.closed {
		t.Errorf("rejected body reads = %d, closed = %v; want untouched", body.reads, body.closed)
	}
}

func TestProviderOutcomeMapping(t *testing.T) {
	tests := []struct {
		name       string
		provider   SnapshotProvider
		wantStatus int
		wantCode   string
		wantFatal  bool
	}{
		{name: "complete", provider: providerFunc(func(context.Context) (snapshot.Encoded, error) {
			return encodedCompleteSnapshot(t), nil
		}), wantStatus: http.StatusOK},
		{name: "partial", provider: providerFunc(func(context.Context) (snapshot.Encoded, error) {
			return encodedPartialSnapshot(t), nil
		}), wantStatus: http.StatusOK},
		{name: "no useful metrics", provider: unavailableProvider{}, wantStatus: http.StatusServiceUnavailable, wantCode: "temporarily_unavailable"},
		{name: "internal error", provider: providerFunc(func(context.Context) (snapshot.Encoded, error) {
			return snapshot.Encoded{}, errors.New("defect")
		}), wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantFatal: true},
		{name: "invalid encoded result", provider: providerFunc(func(context.Context) (snapshot.Encoded, error) {
			return snapshot.Encoded{}, nil
		}), wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantFatal: true},
		{name: "provider panic", provider: providerFunc(func(context.Context) (snapshot.Encoded, error) {
			panic("defect")
		}), wantStatus: http.StatusInternalServerError, wantCode: "internal_error", wantFatal: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fatalCalls := atomic.Int32{}
			handler, err := newRequestHandler(test.provider, func(error) { fatalCalls.Add(1) })
			if err != nil {
				t.Fatalf("newRequestHandler() error = %v", err)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, snapshotPath, nil))
			if response.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantCode != "" && !bytes.Contains(response.Body.Bytes(), []byte(`"code":"`+test.wantCode+`"`)) {
				t.Errorf("body = %s, want code %q", response.Body.Bytes(), test.wantCode)
			}
			wantFatalCalls := int32(0)
			if test.wantFatal {
				wantFatalCalls = 1
			}
			if got := fatalCalls.Load(); got != wantFatalCalls {
				t.Errorf("fatal calls = %d, want %d", got, wantFatalCalls)
			}
		})
	}
}

func TestAdmissionRejectsNinthRequestWithoutQueueing(t *testing.T) {
	entered := make(chan struct{}, maximumAdmittedRequests)
	release := make(chan struct{})
	provider := providerFunc(func(context.Context) (snapshot.Encoded, error) {
		entered <- struct{}{}
		<-release
		return encodedCompleteSnapshot(t), nil
	})
	handler := mustHandler(t, provider)
	done := make(chan struct{}, maximumAdmittedRequests)
	for range maximumAdmittedRequests {
		go func() {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, snapshotPath, nil))
			done <- struct{}{}
		}()
	}
	for range maximumAdmittedRequests {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("timed out filling admission capacity")
		}
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, snapshotPath, nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("overload status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	close(release)
	for range maximumAdmittedRequests {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timed out draining admitted requests")
		}
	}
}

func TestRequestDeadlineCancelsProvider(t *testing.T) {
	provider := providerFunc(func(ctx context.Context) (snapshot.Encoded, error) {
		<-ctx.Done()
		return snapshot.Encoded{}, ctx.Err()
	})
	handler, err := newRequestHandlerWithDeadline(provider, func(error) {}, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("newRequestHandlerWithDeadline() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, snapshotPath, nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Errorf("deadline status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestInternalFailureClosesAdmissionAndNotifiesOnce(t *testing.T) {
	fatalCalls := atomic.Int32{}
	handler := mustHandlerWithFatal(t, providerFunc(func(context.Context) (snapshot.Encoded, error) {
		return snapshot.Encoded{}, errors.New("defect")
	}), func(error) { fatalCalls.Add(1) })
	for range 2 {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, snapshotPath, nil))
		if response.Code != http.StatusInternalServerError && response.Code != http.StatusServiceUnavailable {
			t.Errorf("status after defect = %d, want 500 or 503", response.Code)
		}
	}
	if got := fatalCalls.Load(); got != 1 {
		t.Errorf("fatal calls = %d, want 1", got)
	}
}

func TestHeadResponseHasNoBodyOverHTTP(t *testing.T) {
	server, err := Listen(context.Background(), netip.MustParseAddr("127.0.0.1"), 0, unavailableProvider{}, func(error) {}, io.Discard)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	done := serveInBackground(server)
	defer closeServer(t, server, done)
	request, _ := http.NewRequest(http.MethodHead, "http://"+server.address().String()+snapshotPath, nil)
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatalf("HEAD error = %v", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != "GET" || len(body) != 0 {
		t.Errorf("HEAD response = status %d, Allow %q, body %q", response.StatusCode, response.Header.Get("Allow"), body)
	}
}

type providerFunc func(context.Context) (snapshot.Encoded, error)

func (provider providerFunc) Snapshot(ctx context.Context) (snapshot.Encoded, error) {
	return provider(ctx)
}

type unavailableProvider struct{}

func (unavailableProvider) Snapshot(context.Context) (snapshot.Encoded, error) {
	return snapshot.Encoded{}, snapshot.ErrNoUsefulSnapshot
}

type observedBody struct {
	reads  int
	closed bool
}

func (b *observedBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (b *observedBody) Close() error {
	b.closed = true
	return nil
}

func mustHandler(t *testing.T, provider SnapshotProvider) http.Handler {
	return mustHandlerWithFatal(t, provider, func(error) {})
}

func mustHandlerWithFatal(t *testing.T, provider SnapshotProvider, fatal func(error)) http.Handler {
	t.Helper()
	handler, err := newRequestHandler(provider, fatal)
	if err != nil {
		t.Fatalf("newRequestHandler() error = %v", err)
	}
	return handler
}

func encodedCompleteSnapshot(t *testing.T) snapshot.Encoded {
	t.Helper()
	value := uint64(1)
	load := 0.1
	temperature := 42.5
	state := "up"
	flag := false
	input := snapshot.Snapshot{
		ObservedAt: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC), Host: snapshot.Host{Hostname: "raspberrypi"},
		CPU:            snapshot.CPU{UtilizationPercent: &load, LogicalCPUCount: &value},
		Load:           snapshot.Load{OneMinute: &load, FiveMinutes: &load, FifteenMinutes: &load},
		Memory:         snapshot.Memory{TotalBytes: &value, AvailableBytes: &value, UsedBytes: pointerTo(uint64(0))},
		RootFilesystem: snapshot.RootFilesystem{TotalBytes: &value, AvailableBytes: &value, UsedBytes: pointerTo(uint64(0))},
		UptimeSeconds:  &value,
		Network:        &snapshot.Network{Interfaces: []snapshot.NetworkInterface{{Name: "eth0", State: &state, RXBytes: &value, TXBytes: &value}}},
		RaspberryPi: snapshot.RaspberryPi{SoCTemperatureCelsius: &temperature, ThermalThrottlingActive: &flag,
			ThermalThrottlingOccurredSinceBoot: &flag, UndervoltageActive: &flag, UndervoltageOccurredSinceBoot: &flag},
		Issues: []snapshot.Issue{},
	}
	encoded, err := snapshot.Encode(input)
	if err != nil {
		t.Fatalf("snapshot.Encode() error = %v", err)
	}
	return encoded
}

func encodedPartialSnapshot(t *testing.T) snapshot.Encoded {
	t.Helper()
	value := uint64(1)
	input := snapshot.Snapshot{
		ObservedAt: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC), Host: snapshot.Host{Hostname: "raspberrypi"},
		UptimeSeconds: &value,
		Issues: []snapshot.Issue{
			transportIssue("/cpu/logical_cpu_count"), transportIssue("/load"), transportIssue("/memory"),
			transportIssue("/root_filesystem"), transportIssue("/raspberry_pi/soc_temperature_celsius"),
			transportIssue("/raspberry_pi/thermal_throttling_active"), transportIssue("/raspberry_pi/thermal_throttling_occurred_since_boot"),
			transportIssue("/raspberry_pi/undervoltage_active"), transportIssue("/raspberry_pi/undervoltage_occurred_since_boot"),
			transportIssue("/network"), transportIssue("/cpu/utilization_percent"),
		},
	}
	encoded, err := snapshot.Encode(input)
	if err != nil {
		t.Fatalf("snapshot.Encode() error = %v", err)
	}
	return encoded
}

func transportIssue(path string) snapshot.Issue {
	return snapshot.Issue{Path: path, Code: snapshot.IssueTemporarilyUnavailable, Message: "The metric is temporarily unavailable."}
}

func pointerTo[T any](value T) *T { return &value }
