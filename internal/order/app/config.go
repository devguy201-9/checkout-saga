package app

import "github.com/devguy201-9/checkout-saga/pkg/config"

// Config for the order service: composes the shared blocks (App, Log, PG) plus
// order-specific fields. Per-service config lives in the app layer (composition
// root), not in pkg/config, because ORDER_HTTP_PORT / ORDER_DB_NAME are only
// needed by the order service.
type Config struct {
	config.App
	config.Log
	config.PG

	HTTPPort string `env:"ORDER_HTTP_PORT" envDefault:"8081" validate:"required"`
	DBName   string `env:"ORDER_DB_NAME" envDefault:"order_db" validate:"required"`
}
