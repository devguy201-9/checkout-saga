package postgres

import "time"

// Option configures Postgres via the functional options pattern (like
// pkg/httpserver). Defaults are set in New before options are applied.
type Option func(*Postgres)

// MaxConns sets the maximum number of connections in the pool.
func MaxConns(n int32) Option {
	return func(p *Postgres) { p.maxConns = n }
}

// MinConns sets the minimum number of idle connections kept warm.
func MinConns(n int32) Option {
	return func(p *Postgres) { p.minConns = n }
}

// ConnAttempts sets the number of connection attempts before failing.
func ConnAttempts(n int) Option {
	return func(p *Postgres) { p.connAttempts = n }
}

// ConnTimeout sets the timeout for each connectivity-verifying Ping.
func ConnTimeout(d time.Duration) Option {
	return func(p *Postgres) { p.connTimeout = d }
}

// RetryBackoff sets the initial backoff between retries (doubled each time).
func RetryBackoff(d time.Duration) Option {
	return func(p *Postgres) { p.retryBackoff = d }
}

// MaxConnLifetime sets the maximum lifetime of a single connection.
func MaxConnLifetime(d time.Duration) Option {
	return func(p *Postgres) { p.maxConnLifetime = d }
}

// MaxConnIdleTime sets the maximum idle time before a connection is closed.
func MaxConnIdleTime(d time.Duration) Option {
	return func(p *Postgres) { p.maxConnIdleTime = d }
}

// WithLogger injects a logger used to log each retry attempt. Defaults to no-op.
func WithLogger(l Logger) Option {
	return func(p *Postgres) {
		if l != nil {
			p.logger = l
		}
	}
}
