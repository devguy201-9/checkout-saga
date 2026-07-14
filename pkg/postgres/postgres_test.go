package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeLogger counts how many times Warn is called (one Warn per failed attempt).
type fakeLogger struct {
	warns int
}

func (f *fakeLogger) Info(string, ...zap.Field)  {}
func (f *fakeLogger) Warn(string, ...zap.Field)  { f.warns++ }
func (f *fakeLogger) Error(string, ...zap.Field) {}

func TestOptions_Apply(t *testing.T) {
	t.Parallel()

	p := &Postgres{}
	opts := []Option{
		MaxConns(50),
		MinConns(3),
		ConnAttempts(7),
		ConnTimeout(2 * time.Second),
		RetryBackoff(250 * time.Millisecond),
		MaxConnLifetime(2 * time.Hour),
		MaxConnIdleTime(15 * time.Minute),
		WithLogger(&fakeLogger{}),
	}
	for _, opt := range opts {
		opt(p)
	}

	if p.maxConns != 50 {
		t.Errorf("maxConns = %d, want 50", p.maxConns)
	}
	if p.minConns != 3 {
		t.Errorf("minConns = %d, want 3", p.minConns)
	}
	if p.connAttempts != 7 {
		t.Errorf("connAttempts = %d, want 7", p.connAttempts)
	}
	if p.connTimeout != 2*time.Second {
		t.Errorf("connTimeout = %v, want 2s", p.connTimeout)
	}
	if p.retryBackoff != 250*time.Millisecond {
		t.Errorf("retryBackoff = %v, want 250ms", p.retryBackoff)
	}
	if p.maxConnLifetime != 2*time.Hour {
		t.Errorf("maxConnLifetime = %v, want 2h", p.maxConnLifetime)
	}
	if p.maxConnIdleTime != 15*time.Minute {
		t.Errorf("maxConnIdleTime = %v, want 15m", p.maxConnIdleTime)
	}
	if p.logger == nil {
		t.Error("logger = nil, want non-nil")
	}
}

func TestWithLogger_NilIsIgnored(t *testing.T) {
	t.Parallel()

	p := &Postgres{logger: zap.NewNop()}
	WithLogger(nil)(p)

	if p.logger == nil {
		t.Error("WithLogger(nil) overwrote logger to nil, want kept non-nil")
	}
}

// TestNew_FailsFastAfterRetries verifies: when the DB is unreachable it retries
// the right number of times, logs each attempt, then returns an error (no hang).
// No real DB required.
func TestNew_FailsFastAfterRetries(t *testing.T) {
	t.Parallel()

	fake := &fakeLogger{}
	const attempts = 3

	start := time.Now()
	// 127.0.0.1:1 — nothing listens there -> connection refused.
	pg, err := New(
		context.Background(), "postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		WithLogger(fake),
		ConnAttempts(attempts),
		ConnTimeout(200*time.Millisecond),
		RetryBackoff(10*time.Millisecond),
	)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("New() err = nil, want non-nil when DB unreachable")
	}
	if pg != nil {
		t.Errorf("New() pg = %v, want nil on error", pg)
	}
	if fake.warns != attempts {
		t.Errorf("logged %d attempts, want %d", fake.warns, attempts)
	}
	if elapsed > 5*time.Second {
		t.Errorf("New() took %v, fail-fast too slow", elapsed)
	}
}

func TestNew_CtxCancelledDuringBackoff(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := New(
		ctx, "postgres://u:p@127.0.0.1:1/db?sslmode=disable",
		ConnAttempts(10),
		ConnTimeout(100*time.Millisecond),
		RetryBackoff(500*time.Millisecond),
	)
	if err == nil {
		t.Fatal("New() err = nil, want error from cancelled ctx")
	}
}

func TestHealth_NilPool(t *testing.T) {
	t.Parallel()

	p := &Postgres{}
	if err := p.Health(context.Background()); !errors.Is(err, ErrPoolNotInitialized) {
		t.Errorf("Health() err = %v, want ErrPoolNotInitialized", err)
	}
}

func TestVersion_NilPool(t *testing.T) {
	t.Parallel()

	p := &Postgres{}
	if _, err := p.Version(context.Background()); !errors.Is(err, ErrPoolNotInitialized) {
		t.Errorf("Version() err = %v, want ErrPoolNotInitialized", err)
	}
}

func TestStat_NilPool(t *testing.T) {
	t.Parallel()

	p := &Postgres{}
	if stat := p.Stat(); stat != nil {
		t.Errorf("Stat() = %v, want nil", stat)
	}
}

func TestClose_NilPool(t *testing.T) {
	t.Parallel()

	p := &Postgres{}
	p.Close() // passing means it did not panic
}
