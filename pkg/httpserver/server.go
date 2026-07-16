// Package httpserver wraps net/http.Server with functional options and a
// graceful shutdown, mirroring the style of pkg/postgres.
//
// It carries no business logic and no routing: the caller passes a ready
// http.Handler. That keeps the package reusable by any service that needs to
// expose HTTP.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	_defaultAddr = ":8080"
	// ReadHeaderTimeout guards against Slowloris: a client trickling headers
	// forever would otherwise hold a connection open.
	_defaultReadHeaderTimeout = 5 * time.Second
	_defaultReadTimeout       = 10 * time.Second
	_defaultWriteTimeout      = 10 * time.Second
	_defaultShutdownTimeout   = 10 * time.Second
)

// Server owns an http.Server plus the channel that reports a fatal serve error.
type Server struct {
	server          *http.Server
	notify          chan error
	shutdownTimeout time.Duration
}

// New builds the server. It does not listen yet — call Start.
func New(handler http.Handler, opts ...Option) *Server {
	s := &Server{
		server: &http.Server{
			Addr:              _defaultAddr,
			Handler:           handler,
			ReadHeaderTimeout: _defaultReadHeaderTimeout,
			ReadTimeout:       _defaultReadTimeout,
			WriteTimeout:      _defaultWriteTimeout,
		},
		notify:          make(chan error, 1),
		shutdownTimeout: _defaultShutdownTimeout,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Start serves in a goroutine. A fatal error is delivered on Notify(); a normal
// Shutdown is not an error and is filtered out here.
func (s *Server) Start() {
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.notify <- fmt.Errorf("httpserver.Start: %w", err)
		}
		close(s.notify)
	}()
}

// Notify reports a fatal serve error (e.g. the port is already taken) so the
// composition root can fail instead of silently serving nothing.
func (s *Server) Notify() <-chan error { return s.notify }

// Addr is the listen address, useful for logging.
func (s *Server) Addr() string { return s.server.Addr }

// Shutdown stops accepting new connections and waits for in-flight requests,
// bounded by shutdownTimeout.
//
// It deliberately builds its own context from Background instead of taking the
// caller's: shutdown is normally triggered by a cancelled signal context, and
// deriving from an already-cancelled context would abort the drain immediately.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("httpserver.Shutdown: %w", err)
	}

	return nil
}
