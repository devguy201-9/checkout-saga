# ============================================================
# Database migrations (golang-migrate)
# ============================================================
# Usage: add `include makefile-day2.mk` to the existing Makefile, AFTER the
# `include .env / export` block so the POSTGRES_* variables are passed down here.
# (Or copy the block below to the end of the Makefile.)
#
# Install the CLI once:
#   go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
# (make sure $(go env GOPATH)/bin is on your PATH)

# Default service to migrate. Override: make migrate-up SERVICE=inventory
SERVICE ?= order

# Reuse the same POSTGRES_* credentials from .env — do NOT hardcode
# postgres:postgres. Fallbacks let it run even when .env is not exported.
# One DB per service: order_db, inventory_db, payment_db, saga_db.
PG_HOST ?= $(if $(POSTGRES_HOST),$(POSTGRES_HOST),localhost)
PG_PORT ?= $(if $(POSTGRES_PORT),$(POSTGRES_PORT),5432)
PG_USER ?= $(if $(POSTGRES_USER),$(POSTGRES_USER),checkout)
PG_PASS ?= $(if $(POSTGRES_PASSWORD),$(POSTGRES_PASSWORD),checkout_dev_pwd)
PG_SSL  ?= $(if $(POSTGRES_SSL_MODE),$(POSTGRES_SSL_MODE),disable)

# Override MIGRATE_DB_URL via env for CI/prod.
MIGRATE_DB_URL ?= postgres://$(PG_USER):$(PG_PASS)@$(PG_HOST):$(PG_PORT)/$(SERVICE)_db?sslmode=$(PG_SSL)
MIGRATIONS_DIR := migrations/$(SERVICE)
MIGRATE := migrate -path $(MIGRATIONS_DIR) -database "$(MIGRATE_DB_URL)"

.PHONY: migrate-up migrate-down migrate-down-all migrate-create migrate-force migrate-version

migrate-up: ## Apply all pending migrations (default SERVICE=order)
	@echo "migrate up [$(SERVICE)] <- $(MIGRATIONS_DIR)"
	@$(MIGRATE) up

migrate-down: ## Roll back the most recent migration
	@echo "migrate down 1 [$(SERVICE)]"
	@$(MIGRATE) down 1

migrate-down-all: ## Roll back everything (careful — drops the whole schema)
	@$(MIGRATE) down -all

migrate-create: ## Create a new migration: make migrate-create NAME=add_xxx
	@test -n "$(NAME)" || (echo "ERROR: NAME required, e.g. make migrate-create NAME=add_index" && exit 1)
	@migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)

migrate-force: ## Force a version to clear a dirty state: make migrate-force VERSION=1
	@test -n "$(VERSION)" || (echo "ERROR: VERSION required" && exit 1)
	@$(MIGRATE) force $(VERSION)

migrate-version: ## Print the current migration version
	@$(MIGRATE) version
