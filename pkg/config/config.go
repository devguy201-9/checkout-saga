// Package config provides the shared config blocks used by every service
// (App, Log, PG) plus a generic loader that parses env vars and validates
// struct tags.
//
// Per-service config (e.g. the order service's Config) composes these blocks
// and lives in internal/<service>/app — pkg/config only holds what is reusable
// by 2+ services (the DRY rule: extract only when multiple services need it).
package config

import (
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

// App holds shared service-level metadata.
//
// Env follows the vocabulary in .env.example (APP_ENV=development).
type App struct {
	Name string `env:"APP_NAME" envDefault:"checkout-saga"`
	Env  string `env:"APP_ENV" envDefault:"development" validate:"oneof=development staging production"`
}

// Log configures the logger (pkg/logger reads these fields).
type Log struct {
	Level string `env:"LOG_LEVEL" envDefault:"info" validate:"oneof=debug info warn error"`
}

// PG holds the shared PostgreSQL connection settings. It reuses the same
// POSTGRES_* variables already present in .env — it does NOT
// invent a separate PG_URL.
//
// The concrete database name (order_db, payment_db, ...) is per-service and so
// is NOT stored here; the service passes dbName into DSN(). The retry/timeout/
// lifetime parameters use the defaults in pkg/postgres (YAGNI — add env vars
// only when tuning is actually needed).
type PG struct {
	Host     string `env:"POSTGRES_HOST" envDefault:"localhost" validate:"required"`
	Port     int    `env:"POSTGRES_PORT" envDefault:"5432" validate:"gte=1,lte=65535"`
	User     string `env:"POSTGRES_USER,required" validate:"required"`
	Password string `env:"POSTGRES_PASSWORD,required" validate:"required"`
	SSLMode  string `env:"POSTGRES_SSL_MODE" envDefault:"disable" validate:"oneof=disable require verify-ca verify-full"`
	PoolMax  int32  `env:"POSTGRES_POOL_MAX" envDefault:"25" validate:"gte=1,lte=100"`
}

// DSN builds the pgx connection string for a specific database.
func (p PG) DSN(dbName string) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		p.User, p.Password, p.Host, p.Port, dbName, p.SSLMode,
	)
}

// Load parses environment variables into T, then validates by struct tags.
//
// T is typically a per-service config struct composed from App, Log and PG.
// It returns a clearly scoped error for both env-parse and validation failures
// so the caller can fail-fast at startup.
func Load[T any]() (*T, error) {
	cfg := new(T)

	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("config.Load: parse env: %w", err)
	}

	// Build the validator locally instead of using a package-level global:
	// avoids global mutable state (gochecknoglobals) and suits DI. Config is
	// loaded once at startup, so the construction cost is negligible.
	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config.Load: validate: %w", err)
	}

	return cfg, nil
}
