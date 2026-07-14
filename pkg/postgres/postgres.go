// Package postgres provides a connection-pool wrapper around pgx/v5/pgxpool:
// an options pattern, connect retry with exponential backoff (logging each
// attempt), a health check, and reading the server version / pool stats.
//
// This wrapper is shared by every service that has a DB (order, inventory,
// payment, saga), so it lives in pkg/. It contains no business logic and knows
// nothing about the domain.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

// Default pool values used when the caller passes no option.
const (
	_defaultMaxConns        = int32(25)
	_defaultMinConns        = int32(5)
	_defaultConnAttempts    = 5
	_defaultConnTimeout     = 5 * time.Second
	_defaultRetryBackoff    = 1 * time.Second
	_defaultMaxConnLifetime = time.Hour
	_defaultMaxConnIdleTime = 30 * time.Minute
)

// ErrPoolNotInitialized is returned when a method is called before connecting.
var ErrPoolNotInitialized = errors.New("postgres: pool not initialized")

// Logger is the minimal logging surface pkg/postgres needs (a consumer-side
// interface, the Go idiom). Both *zap.Logger and any wrapper that embeds it
// satisfy this interface, so there is no hard coupling to pkg/logger.
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
}

// Postgres wraps a pgxpool.Pool together with its pool config and logger.
type Postgres struct {
	Pool *pgxpool.Pool

	maxConns        int32
	minConns        int32
	connAttempts    int
	connTimeout     time.Duration
	retryBackoff    time.Duration
	maxConnLifetime time.Duration
	maxConnIdleTime time.Duration
	logger          Logger
}

// New creates the pool and verifies connectivity with Ping, retrying with
// exponential backoff on failure. It returns a (wrapped) error if it still
// fails after connAttempts tries — the caller uses this to fail-fast at startup.
func New(ctx context.Context, url string, opts ...Option) (*Postgres, error) {
	pg := &Postgres{
		maxConns:        _defaultMaxConns,
		minConns:        _defaultMinConns,
		connAttempts:    _defaultConnAttempts,
		connTimeout:     _defaultConnTimeout,
		retryBackoff:    _defaultRetryBackoff,
		maxConnLifetime: _defaultMaxConnLifetime,
		maxConnIdleTime: _defaultMaxConnIdleTime,
		logger:          zap.NewNop(),
	}
	for _, opt := range opts {
		opt(pg)
	}

	poolCfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("postgres.New: parse config: %w", err)
	}
	poolCfg.MaxConns = pg.maxConns
	// pgx requires MinConns <= MaxConns. If MaxConns is small (e.g. a low
	// POSTGRES_POOL_MAX), lower MinConns to MaxConns to avoid a config error.
	if pg.minConns > pg.maxConns {
		pg.minConns = pg.maxConns
	}
	poolCfg.MinConns = pg.minConns
	poolCfg.MaxConnLifetime = pg.maxConnLifetime
	poolCfg.MaxConnIdleTime = pg.maxConnIdleTime

	pool, err := pg.connectWithRetry(ctx, poolCfg)
	if err != nil {
		return nil, err
	}

	pg.Pool = pool
	return pg, nil
}

// connectWithRetry tries to open the pool + Ping, retrying with exponential
// backoff. Split out of New to keep per-function complexity low and testable.
func (p *Postgres) connectWithRetry(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	backoff := p.retryBackoff

	var lastErr error
	for attempt := 1; attempt <= p.connAttempts; attempt++ {
		pool, err := p.tryConnect(ctx, cfg)
		if err == nil {
			return pool, nil
		}
		lastErr = err

		// Log every attempt — this is the "what happens when the DB is down" path.
		p.logger.Warn(
			"postgres connect attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", p.connAttempts),
			zap.Duration("next_backoff", backoff),
			zap.Error(err),
		)

		if attempt == p.connAttempts {
			break
		}

		select {
		case <-time.After(backoff):
			backoff *= 2 // exponential
		case <-ctx.Done():
			return nil, fmt.Errorf("postgres.connectWithRetry: ctx cancelled: %w", ctx.Err())
		}
	}

	return nil, fmt.Errorf("postgres.connectWithRetry: failed after %d attempts: %w", p.connAttempts, lastErr)
}

// tryConnect opens the pool and Pings once (with a timeout). The Ping is
// required because pgxpool.NewWithConfig is lazy — it does not actually connect
// until the first query/ping.
func (p *Postgres) tryConnect(ctx context.Context, cfg *pgxpool.Config) (*pgxpool.Pool, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, p.connTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close() // close the half-open pool so connections do not leak
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}

// Health pings the DB for health/readiness probes. Returns ErrPoolNotInitialized
// if the pool is not ready.
func (p *Postgres) Health(ctx context.Context) error {
	if p.Pool == nil {
		return ErrPoolNotInitialized
	}
	if err := p.Pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres.Health: %w", err)
	}
	return nil
}

// Version reads "SELECT version()" — logged at startup to confirm the server.
func (p *Postgres) Version(ctx context.Context) (string, error) {
	if p.Pool == nil {
		return "", ErrPoolNotInitialized
	}

	var version string
	if err := p.Pool.QueryRow(ctx, "SELECT version()").Scan(&version); err != nil {
		return "", fmt.Errorf("postgres.Version: %w", err)
	}
	return version, nil
}

// Stat returns pool statistics (total/idle/acquired conns, ...) for logs/metrics.
func (p *Postgres) Stat() *pgxpool.Stat {
	if p.Pool == nil {
		return nil
	}
	return p.Pool.Stat()
}

// Close closes the pool. Safe to call on a pool that was never initialized.
func (p *Postgres) Close() {
	if p.Pool != nil {
		p.Pool.Close()
	}
}
