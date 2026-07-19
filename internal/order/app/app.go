// Package app is the composition root of the order service: it loads config,
// builds the logger, connects to the database (retry + fail-fast), and manages
// the lifecycle / graceful shutdown.
package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	orderhttp "github.com/devguy201-9/checkout-saga/internal/order/controller/http"
	"github.com/devguy201-9/checkout-saga/internal/order/repo"
	"github.com/devguy201-9/checkout-saga/internal/order/usecase"
	"github.com/devguy201-9/checkout-saga/pkg/config"
	"github.com/devguy201-9/checkout-saga/pkg/httpserver"
	"github.com/devguy201-9/checkout-saga/pkg/jwt"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
	"github.com/devguy201-9/checkout-saga/pkg/postgres"
)

const serviceName = "order-service"

// Run starts the order service. It returns an error so main can decide the exit
// code — keeping os.Exit in main.go (no exit from deep in the code).
func Run(version, commit string) error {
	// Load .env if present (development). Production uses real env vars.
	// godotenv does NOT override already-set vars (e.g. exported by the Makefile).
	if err := godotenv.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: .env not loaded: %v\n", err)
	}

	cfg, err := config.Load[Config]()
	if err != nil {
		return fmt.Errorf("app.Run: load config: %w", err)
	}

	log := logger.NewWithService(cfg.Log.Level, serviceName, cfg.App.Env == "development")
	defer func() {
		if err := log.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "logger sync failed: %v\n", err)
		}
	}()

	// Graceful shutdown: SIGINT/SIGTERM cancels ctx, which propagates to all DB calls.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info(
		"order service starting",
		zap.String("version", version),
		zap.String("commit", commit),
		zap.String("env", cfg.App.Env),
	)

	pg, err := connectDB(ctx, log, cfg)
	if err != nil {
		// Fail-fast: if the DB is not up after retries, the service cannot serve.
		log.Error("postgres connect failed after retries, shutting down", zap.Error(err))
		return fmt.Errorf("app.Run: %w", err)
	}
	defer pg.Close()

	logDBInfo(ctx, log, pg)
	// Wire the layers: repo (data) -> usecase (business) -> controller (HTTP).
	// Dependencies flow inward and are injected through constructors, so each
	// layer can be tested with a fake of the layer below.
	orderRepo := repo.NewOrderRepo(pg)
	orderUseCase := usecase.NewOrderUseCase(orderRepo)
	// Built from config (secret validated min=32 at load), injected — no globals.
	jwtMgr := jwt.NewManager(cfg.Auth.JWTSecret, cfg.Auth.JWTExpiry)
	router := orderhttp.NewRouter(orderUseCase, jwtMgr, log)

	server := httpserver.New(router, httpserver.Port(cfg.HTTPPort))
	server.Start()
	log.Info("order service ready", zap.String("http_port", cfg.HTTPPort))

	return waitForShutdown(ctx, log, server)
}

// waitForShutdown blocks until either a signal or a fatal server error, then
// drains HTTP. The DB pool is closed after this returns (deferred in Run), so
// in-flight requests still have a working pool while they finish.
func waitForShutdown(ctx context.Context, log logger.Logger, server *httpserver.Server) error {
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received, draining http server")

	case err := <-server.Notify():
		// e.g. the port is already taken — do not pretend to be healthy.
		log.Error("http server stopped unexpectedly", zap.Error(err))

		return fmt.Errorf("app.Run: %w", err)
	}

	if err := server.Shutdown(); err != nil {
		log.Error("http server shutdown", zap.Error(err))

		return fmt.Errorf("app.Run: %w", err)
	}

	log.Info("shutdown complete, exiting gracefully")

	return nil
}

// connectDB builds the pool from cfg and routes retry logs through the service
// logger. Only MaxConns is passed (from POSTGRES_POOL_MAX); the retry/timeout/
// lifetime parameters use the defaults in pkg/postgres.
func connectDB(ctx context.Context, log logger.Logger, cfg *Config) (*postgres.Postgres, error) {
	pg, err := postgres.New(
		ctx, cfg.PG.DSN(cfg.DBName),
		postgres.WithLogger(log),
		postgres.MaxConns(cfg.PG.PoolMax),
	)
	if err != nil {
		return nil, fmt.Errorf("connectDB: %w", err)
	}
	return pg, nil
}

// logDBInfo logs the server version + pool stats to confirm a successful connect.
func logDBInfo(ctx context.Context, log logger.Logger, pg *postgres.Postgres) {
	dbVersion, err := pg.Version(ctx)
	if err != nil {
		log.Warn("could not read postgres version", zap.Error(err))
		dbVersion = "unknown"
	}

	stat := pg.Stat()
	if stat == nil {
		log.Warn("pool stat unavailable")
		return
	}

	log.Info(
		"postgres connected",
		zap.String("db_version", dbVersion),
		zap.Int32("pool_max_conns", stat.MaxConns()),
		zap.Int32("pool_total_conns", stat.TotalConns()),
		zap.Int32("pool_idle_conns", stat.IdleConns()),
	)
}
