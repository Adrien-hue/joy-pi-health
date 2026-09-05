// Package httpapi owns Joy Pi Health's HTTP transport lifecycle.
package httpapi

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

const (
	maximumConnections = 16
	maximumHeaderBytes = 8 * 1024
	headerReadTimeout  = 2 * time.Second
	readTimeout        = 2 * time.Second
	idleTimeout        = 15 * time.Second
	writeTimeout       = time.Second
)

// Server owns one bound listener and its HTTP server.
type Server struct {
	httpServer *http.Server
	listener   net.Listener
}

// Listen binds a bounded IPv4 HTTP server to the configured endpoint.
func Listen(ctx context.Context, address netip.Addr, port uint16, provider SnapshotProvider, fatal func(error), errorOutput io.Writer) (*Server, error) {
	handler, err := newRequestHandler(provider, fatal)
	if err != nil {
		return nil, err
	}
	endpoint := net.JoinHostPort(address.String(), strconv.FormatUint(uint64(port), 10))
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp4", endpoint)
	if err != nil {
		return nil, err
	}

	return newServer(listener, handler, errorOutput), nil
}

func newServer(listener net.Listener, handler http.Handler, errorOutput io.Writer) *Server {
	return &Server{
		httpServer: &http.Server{
			Handler:           handler,
			ReadTimeout:       readTimeout,
			ReadHeaderTimeout: headerReadTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			MaxHeaderBytes:    maximumHeaderBytes,
			ErrorLog:          log.New(errorOutput, "joy-pi-health: http: ", 0),
		},
		listener: newLimitedListener(listener, maximumConnections),
	}
}

// Serve accepts HTTP connections until the server is shut down or fails.
func (s *Server) Serve() error {
	err := s.httpServer.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

// Shutdown closes the listener and gracefully drains active connections.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// Close immediately closes the listener and all active connections.
func (s *Server) Close() error {
	return s.httpServer.Close()
}

func (s *Server) address() net.Addr {
	return s.listener.Addr()
}

type limitedListener struct {
	net.Listener
	slots     chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func newLimitedListener(listener net.Listener, limit int) *limitedListener {
	return &limitedListener{
		Listener: listener,
		slots:    make(chan struct{}, limit),
		closed:   make(chan struct{}),
	}
}

func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case l.slots <- struct{}{}:
	case <-l.closed:
		return nil, net.ErrClosed
	}

	connection, err := l.Listener.Accept()
	if err != nil {
		<-l.slots
		return nil, err
	}
	return &limitedConnection{Conn: connection, release: func() { <-l.slots }}, nil
}

func (l *limitedListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return l.Listener.Close()
}

type limitedConnection struct {
	net.Conn
	releaseOnce sync.Once
	release     func()
}

func (c *limitedConnection) Close() error {
	err := c.Conn.Close()
	c.releaseOnce.Do(c.release)
	return err
}
