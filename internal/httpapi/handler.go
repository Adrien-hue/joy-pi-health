package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Adrien-hue/joy-pi-health/internal/snapshot"
)

const (
	snapshotPath            = "/v1/snapshot"
	maximumAdmittedRequests = 8
	requestDeadline         = time.Second
	maximumErrorSize        = 2 * 1024
)

// SnapshotProvider supplies a finalized current snapshot. A future collection
// coordinator will implement this boundary without coupling HTTP to observation.
type SnapshotProvider interface {
	Snapshot(context.Context) (snapshot.Encoded, error)
}

type requestHandler struct {
	provider  SnapshotProvider
	admission chan struct{}
	deadline  time.Duration
	fatal     func(error)
	fatalOnce sync.Once
	failed    atomic.Bool
	errors    map[string][]byte
}

func newRequestHandler(provider SnapshotProvider, fatal func(error)) (http.Handler, error) {
	return newRequestHandlerWithDeadline(provider, fatal, requestDeadline)
}

func newRequestHandlerWithDeadline(provider SnapshotProvider, fatal func(error), deadline time.Duration) (http.Handler, error) {
	if provider == nil {
		return nil, errors.New("snapshot provider is required")
	}
	if fatal == nil {
		return nil, errors.New("fatal notifier is required")
	}
	if deadline <= 0 {
		return nil, errors.New("request deadline must be positive")
	}
	handler := &requestHandler{
		provider: provider, admission: make(chan struct{}, maximumAdmittedRequests),
		deadline: deadline, fatal: fatal, errors: make(map[string][]byte, 3),
	}
	for code, message := range map[string]string{
		"invalid_request":         "The request is invalid.",
		"temporarily_unavailable": "No useful snapshot is currently available.",
		"internal_error":          "The service encountered an internal error.",
	} {
		body, err := json.Marshal(errorDocument{Error: errorValue{Code: code, Message: message}})
		if err != nil {
			return nil, fmt.Errorf("encode %s response: %w", code, err)
		}
		if len(body) > maximumErrorSize {
			return nil, fmt.Errorf("encoded %s response exceeds %d bytes", code, maximumErrorSize)
		}
		handler.errors[code] = body
	}
	return handler, nil
}

func (h *requestHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	tracked := &trackedResponseWriter{ResponseWriter: response}
	defer func() {
		if recovered := recover(); recovered != nil {
			h.failed.Store(true)
			if !tracked.committed {
				h.writeError(tracked, http.StatusInternalServerError, "internal_error")
			}
			h.fatalOnce.Do(func() { h.fatal(fmt.Errorf("HTTP handler panic: %v", recovered)) })
		}
	}()
	h.serveHTTP(tracked, request)
}

func (h *requestHandler) serveHTTP(response http.ResponseWriter, request *http.Request) {
	if request.URL.EscapedPath() != snapshotPath {
		h.writeError(response, http.StatusNotFound, "invalid_request")
		return
	}
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		h.writeError(response, http.StatusMethodNotAllowed, "invalid_request")
		return
	}
	if request.URL.RawQuery != "" {
		h.writeError(response, http.StatusBadRequest, "invalid_request")
		return
	}
	if len(request.TransferEncoding) != 0 || request.ContentLength != 0 {
		request.Close = true
		response.Header().Set("Connection", "close")
		h.writeError(response, http.StatusBadRequest, "invalid_request")
		return
	}
	if h.failed.Load() {
		h.writeError(response, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	select {
	case h.admission <- struct{}{}:
		defer func() { <-h.admission }()
	default:
		h.writeError(response, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), h.deadline)
	defer cancel()
	encoded, err := h.callProvider(ctx)
	if request.Context().Err() != nil {
		return
	}
	if h.failed.Load() {
		h.writeError(response, http.StatusInternalServerError, "internal_error")
		return
	}
	if err != nil {
		switch {
		case errors.Is(err, snapshot.ErrNoUsefulSnapshot), errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
			h.writeError(response, http.StatusServiceUnavailable, "temporarily_unavailable")
		default:
			h.writeInternalError(response, err)
		}
		return
	}
	if ctx.Err() != nil {
		h.writeError(response, http.StatusServiceUnavailable, "temporarily_unavailable")
		return
	}
	if !encoded.Valid() || encoded.Len() > snapshot.MaximumSize {
		h.writeInternalError(response, errors.New("provider returned an invalid encoded snapshot"))
		return
	}

	setResponseHeaders(response.Header())
	response.WriteHeader(http.StatusOK)
	_, _ = encoded.WriteTo(response)
}

func (h *requestHandler) callProvider(ctx context.Context) (encoded snapshot.Encoded, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("snapshot provider panic: %v", recovered)
		}
	}()
	return h.provider.Snapshot(ctx)
}

func (h *requestHandler) writeInternalError(response http.ResponseWriter, err error) {
	h.failed.Store(true)
	h.writeError(response, http.StatusInternalServerError, "internal_error")
	h.fatalOnce.Do(func() { h.fatal(err) })
}

func (h *requestHandler) writeError(response http.ResponseWriter, status int, code string) {
	setResponseHeaders(response.Header())
	response.WriteHeader(status)
	_, _ = response.Write(h.errors[code])
}

func setResponseHeaders(header http.Header) {
	header.Set("Content-Type", "application/json")
	header.Set("Cache-Control", "no-store")
}

type errorDocument struct {
	Error errorValue `json:"error"`
}

type errorValue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type trackedResponseWriter struct {
	http.ResponseWriter
	committed bool
}

func (w *trackedResponseWriter) WriteHeader(status int) {
	w.committed = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *trackedResponseWriter) Write(body []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}
