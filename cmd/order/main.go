// Order Service entry point.
//
// Skeleton: init logger and wait for SIGTERM.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

const serviceName = "order-service"

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := godotenv.Load(); err != nil {
		// .env optional in prod (env vars set directly) — log at debug/warn, don't fail
		fmt.Fprintf(os.Stderr, "warning: .env not loaded: %v\n", err)
	}

	level := envOr("LOG_LEVEL", "info")
	dev := envOr("APP_ENV", "development") == "development"

	log := logger.NewWithService(level, serviceName, dev)
	defer func() {
		if err := log.Sync(); err != nil {
			fmt.Fprintf(os.Stderr, "logger sync failed: %v\n", err)
		}
	}()

	log.Info(
		"service starting",
		zap.String("version", version),
		zap.String("commit", commit),
		zap.String("go_version", goVersion()),
		zap.String("port", envOr("ORDER_HTTP_PORT", "8081")),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Info("shutdown signal received, exiting gracefully")
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func goVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		return bi.GoVersion
	}
	return "unknown"
}
