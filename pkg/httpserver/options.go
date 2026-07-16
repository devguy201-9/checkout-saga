package httpserver

import (
	"net"
	"time"
)

// Option mutates the Server before it starts (functional options pattern —
// same shape as pkg/postgres, so both packages read the same way).
type Option func(*Server)

// Port sets the listen port, e.g. Port("8081") -> ":8081".
func Port(port string) Option {
	return func(s *Server) {
		s.server.Addr = net.JoinHostPort("", port)
	}
}

// ReadTimeout bounds reading the whole request, body included.
func ReadTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.server.ReadTimeout = d
	}
}

// WriteTimeout bounds writing the response.
func WriteTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.server.WriteTimeout = d
	}
}

// ShutdownTimeout bounds how long Shutdown waits for in-flight requests.
func ShutdownTimeout(d time.Duration) Option {
	return func(s *Server) {
		s.shutdownTimeout = d
	}
}
