package httpapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestListenServesTemporaryNotFound(t *testing.T) {
	server, err := Listen(context.Background(), netip.MustParseAddr("127.0.0.1"), 0, io.Discard)
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	done := serveInBackground(server)
	t.Cleanup(func() { closeServer(t, server, done) })

	response, err := (&http.Client{Timeout: time.Second}).Get("http://" + server.address().String() + "/v1/snapshot")
	if err != nil {
		t.Fatalf("GET /v1/snapshot error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("GET /v1/snapshot status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
}

func TestListenReportsOccupiedPort(t *testing.T) {
	occupied, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer occupied.Close()

	port := occupied.Addr().(*net.TCPAddr).Port
	server, err := Listen(context.Background(), netip.MustParseAddr("127.0.0.1"), uint16(port), io.Discard)
	if err == nil {
		_ = server.Close()
		t.Fatal("Listen() error = nil, want occupied-port error")
	}
}

func TestServerSettings(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	server := newServer(listener, http.NotFoundHandler(), io.Discard)
	defer server.Close()

	if server.httpServer.ReadTimeout != readTimeout {
		t.Errorf("ReadTimeout = %v, want %v", server.httpServer.ReadTimeout, readTimeout)
	}
	if server.httpServer.ReadHeaderTimeout != headerReadTimeout {
		t.Errorf("ReadHeaderTimeout = %v, want %v", server.httpServer.ReadHeaderTimeout, headerReadTimeout)
	}
	if server.httpServer.WriteTimeout != writeTimeout {
		t.Errorf("WriteTimeout = %v, want %v", server.httpServer.WriteTimeout, writeTimeout)
	}
	if server.httpServer.IdleTimeout != idleTimeout {
		t.Errorf("IdleTimeout = %v, want %v", server.httpServer.IdleTimeout, idleTimeout)
	}
	if server.httpServer.MaxHeaderBytes != maximumHeaderBytes {
		t.Errorf("MaxHeaderBytes = %d, want %d", server.httpServer.MaxHeaderBytes, maximumHeaderBytes)
	}
	if capacity := cap(server.listener.(*limitedListener).slots); capacity != maximumConnections {
		t.Errorf("connection limit = %d, want %d", capacity, maximumConnections)
	}
}

func TestShutdownClosesListenerBeforeDraining(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	tracked := &trackingListener{Listener: listener, closed: make(chan struct{})}
	entered := make(chan struct{})
	release := make(chan struct{})
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	})
	server := newServer(tracked, handler, io.Discard)
	done := serveInBackground(server)

	requestDone := make(chan error, 1)
	go func() {
		response, requestErr := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + server.address().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
		requestDone <- requestErr
	}()
	waitFor(t, entered, "handler entry")

	shutdownDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		shutdownDone <- server.Shutdown(ctx)
	}()
	waitFor(t, tracked.closed, "listener close")

	select {
	case shutdownErr := <-shutdownDone:
		t.Fatalf("Shutdown() returned before active handler drained: %v", shutdownErr)
	default:
	}

	close(release)
	if shutdownErr := waitForValue(t, shutdownDone, "shutdown completion"); shutdownErr != nil {
		t.Errorf("Shutdown() error = %v", shutdownErr)
	}
	if serveErr := waitForValue(t, done, "serve completion"); serveErr != nil {
		t.Errorf("Serve() error = %v", serveErr)
	}
	if requestErr := waitForValue(t, requestDone, "request completion"); requestErr != nil {
		t.Errorf("request error = %v", requestErr)
	}
}

func TestCloseCancelsActiveHandler(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		close(entered)
		<-request.Context().Done()
		close(cancelled)
	})
	server := newServer(listener, handler, io.Discard)
	done := serveInBackground(server)

	requestDone := make(chan struct{})
	go func() {
		response, requestErr := (&http.Client{Timeout: 2 * time.Second}).Get("http://" + server.address().String())
		if requestErr == nil {
			_ = response.Body.Close()
		}
		close(requestDone)
	}()
	waitFor(t, entered, "handler entry")

	if err := server.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitFor(t, cancelled, "handler cancellation")
	if serveErr := waitForValue(t, done, "serve completion"); serveErr != nil {
		t.Errorf("Serve() error = %v", serveErr)
	}
	waitFor(t, requestDone, "request completion")
}

func TestLimitedListenerBoundsAcceptedConnections(t *testing.T) {
	underlying := newQueuedListener(3)
	listener := newLimitedListener(underlying, 2)
	defer listener.Close()

	serverConnections := make([]net.Conn, 0, 3)
	clientConnections := make([]net.Conn, 0, 3)
	for range 3 {
		serverConnection, clientConnection := net.Pipe()
		underlying.connections <- serverConnection
		clientConnections = append(clientConnections, clientConnection)
	}
	defer func() {
		for _, connection := range clientConnections {
			_ = connection.Close()
		}
	}()

	for range 2 {
		connection, err := listener.Accept()
		if err != nil {
			t.Fatalf("Accept() error = %v", err)
		}
		serverConnections = append(serverConnections, connection)
	}

	thirdAccepted := make(chan net.Conn, 1)
	go func() {
		connection, _ := listener.Accept()
		thirdAccepted <- connection
	}()
	select {
	case connection := <-thirdAccepted:
		if connection != nil {
			_ = connection.Close()
		}
		t.Fatal("third connection accepted before a slot was released")
	case <-time.After(20 * time.Millisecond):
	}

	_ = serverConnections[0].Close()
	third := waitForValue(t, thirdAccepted, "third connection acceptance")
	if third == nil {
		t.Fatal("third accepted connection = nil")
	}
	serverConnections = append(serverConnections, third)
	for _, connection := range serverConnections[1:] {
		_ = connection.Close()
	}
}

func serveInBackground(server *Server) <-chan error {
	done := make(chan error, 1)
	go func() { done <- server.Serve() }()
	return done
}

func closeServer(t *testing.T, server *Server, done <-chan error) {
	t.Helper()
	if err := server.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
	if err := waitForValue(t, done, "serve completion"); err != nil {
		t.Errorf("Serve() error = %v", err)
	}
}

func waitFor(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForValue[T any](t *testing.T, values <-chan T, description string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
		var zero T
		return zero
	}
}

type trackingListener struct {
	net.Listener
	closed chan struct{}
	once   sync.Once
}

func (l *trackingListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return l.Listener.Close()
}

type queuedListener struct {
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func newQueuedListener(capacity int) *queuedListener {
	return &queuedListener{
		connections: make(chan net.Conn, capacity),
		closed:      make(chan struct{}),
	}
}

func (l *queuedListener) Accept() (net.Conn, error) {
	select {
	case connection := <-l.connections:
		return connection, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *queuedListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *queuedListener) Addr() net.Addr { return testAddress("queued") }

type testAddress string

func (a testAddress) Network() string { return string(a) }
func (a testAddress) String() string  { return string(a) }
