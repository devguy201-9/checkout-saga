# PostgreSQL Connection Layer Checkpoint

**Theme**: order_db migration + `pkg/postgres` (retry/fail-fast) + `pkg/config` + integrate `cmd/order`

## What this covers

**Migration**:
- `migrations/order/000001_orders.up.sql` — `orders` table (idempotency_key UNIQUE, version for optimistic lock, status CHECK per state machine) + `order_items` (FK ON DELETE CASCADE) + indexes.
- `000001_orders.down.sql` — rollback (FK-safe drop order).

**Code**:
- `pkg/config` — shared `App`/`Log`/`PG` blocks + generic `Load[T]()` (caarlos0/env + validator; validator built locally → no global state).
- `pkg/postgres` — pgx/v5 pool wrapper: options pattern, connect retry with exponential backoff (logs each attempt), `Health`/`Version`/`Stat`/`Close`.
- `pkg/postgres/postgres_test.go` — unit tests that need no DB (options, fail-fast + attempt counting, nil-pool guards).
- `internal/order/app` — `Config` composition + `Run` (composition root): load config → logger → connect DB (fail-fast) → log version/stat → graceful shutdown.
- `cmd/order/main.go` — slim, just calls `app.Run` and decides the exit code.

**Docs**:
- `docs/adr/0004-pgx-over-database-sql.md`
- `docs/codebase-summary.md` — status updated.

## Dependencies

`go.mod` now requires `pgx/v5`, `caarlos0/env/v11`, `validator/v10`, `godotenv`,
and `zap`. Pull them and install the migration CLI once:

```bash
go mod tidy

# Migration CLI (install once, NOT a go dependency of the module)
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate -version   # verify the CLI is on PATH ($(go env GOPATH)/bin)
```

## Verify checkpoint — sequential

Run in order; each step must pass before the next.

### 1. Makefile / env

The `.env.example` already has every variable this layer needs (APP_ENV,
LOG_LEVEL, POSTGRES_*, ORDER_DB_NAME, ORDER_HTTP_PORT) → nothing to merge.

```bash
cp .env.example .env   # if you do not have .env yet
```

The Makefile ends with `include makefile-day2.mk` (placed after the
`include .env / export` block so POSTGRES_* propagate to the migrate URL).

Expect: `make help` shows the `migrate-*` targets.

### 2. Start infra (if not running)

```bash
make up
make verify    # 4 DBs exist + PONG
```

### 3. Migrate up order_db

```bash
make migrate-up            # SERVICE=order by default
make migrate-version       # Expect: 1
```

Expect: prints `migrate up [order] <- migrations/order`, no errors.

### 4. Verify schema

```bash
docker exec -it checkout-postgres psql -U checkout -d order_db -c "\d orders"
docker exec -it checkout-postgres psql -U checkout -d order_db -c "\d order_items"
docker exec -it checkout-postgres psql -U checkout -d order_db -c "\di"
```

Expect:
- `orders`: `idempotency_key` UNIQUE, `version` default 0, `status` has a CHECK constraint, `total_amount` CHECK > 0.
- `order_items`: FK `order_id` -> `orders(id)`.
- Indexes: `idx_orders_user_id`, `idx_orders_status`, `idx_order_items_order_id`.

### 5. Run order service — happy path (DB up)

```bash
make run-order
```

Expect log (console when APP_ENV=development; the `service` field is added by `NewWithService`):
```
INFO  order service starting   {"service":"order-service","version":"dev","commit":"unknown","env":"development"}
INFO  postgres connected       {"service":"order-service","db_version":"PostgreSQL 16...","pool_max_conns":25,"pool_total_conns":1,...}
INFO  order service ready      {"service":"order-service","http_port":"8081"}
```
`Ctrl+C`:
```
INFO  shutdown signal received, exiting gracefully
```

### 6. [IMPORTANT] DB down — retry + fail-fast

This is the "what happens when the DB is down?" scenario interviewers like to ask.

```bash
docker compose stop postgres     # stop the DB
make run-order                   # default ConnAttempts=5, RetryBackoff=1s (pkg/postgres)
```

Expect: exactly 5 attempts with increasing backoff (1s, 2s, 4s, 8s), then a non-zero exit code:
```
WARN  postgres connect attempt failed  {"attempt":1,"max_attempts":5,"next_backoff":"1s","error":"...connection refused"}
WARN  postgres connect attempt failed  {"attempt":2,"max_attempts":5,"next_backoff":"2s",...}
...
WARN  postgres connect attempt failed  {"attempt":5,"max_attempts":5,...}
ERROR postgres connect failed after retries, shutting down  {"error":"...failed after 5 attempts..."}
order-service fatal: app.Run: ...
```
```bash
echo $?        # Expect: 1 (fail-fast, no hang)
docker compose start postgres   # bring the DB back up
```

### 7. Migrate down (rollback verify)

```bash
make migrate-down
docker exec -it checkout-postgres psql -U checkout -d order_db -c "\dt"
```

Expect: `orders` + `order_items` are gone (only schema_migrations remains).

```bash
make migrate-up   # bring the schema back to continue
```

### 8. Unit tests (no DB needed)

```bash
make test         # go test -race -cover ./...
go test -race -run TestNew_FailsFastAfterRetries ./pkg/postgres/ -v
```

Expect: `pkg/postgres` PASS, not flaky; `TestNew_FailsFastAfterRetries` logs 3 attempts then fails in < 1s.

### 9. Quality gates

```bash
make fmt          # gofmt/gofumpt — aligns struct tags, etc.
make tidy
make lint         # golangci-lint v2 strict
make vuln
```

Each must pass.

## Commit

```bash
git add .
git commit -m "feat(order): postgres connection layer + order_db schema

- migrations/order 000001: orders + order_items (idempotency UNIQUE,
  version optimistic lock, status CHECK state machine, FK cascade)
- pkg/postgres: pgxpool wrapper, options pattern, connect retry with
  exponential backoff (log each attempt), health/version/stat
- pkg/config: shared App/Log/PG blocks + generic Load[T] (env+validate)
- internal/order/app: composition root — load config, connect DB
  fail-fast after retries, log version+pool stats, graceful shutdown
- cmd/order: slim main delegating to app.Run
- Makefile: migrate-up/down/create/force/version (golang-migrate)
- docs(adr): 0004 pgx/v5 over database/sql"
git tag v0.2.0
git push && git push --tags
```

## Common pitfalls

- **`migrate` not found**: `$(go env GOPATH)/bin` is not on PATH.
- **`Dirty database version`**: a migration failed midway → `make migrate-force VERSION=<n>`, then fix the SQL.
- **`POSTGRES_USER/PASSWORD required`**: no `cp .env.example .env`, or a missing variable → `config.Load` returns a validation error and fails fast (by design).
- **Connect hangs too long**: Ping has its own timeout (`ConnTimeout` default in pkg/postgres), never unbounded.
- **`make lint` reports a gofmt struct-tag diff**: run `make fmt` first (aligns field/type/tag columns).

## Trade-offs chosen

1. **Native pgx/v5 (pgxpool)** instead of `database/sql` + `lib/pq` — ADR-0004. Native transactions for `FOR UPDATE`/outbox, good Postgres type mapping, `Stat()` for metrics.
2. **status: VARCHAR + CHECK** instead of an ENUM type — changing/adding a state needs no `ALTER TYPE`, while still rejecting junk values at the DB level.
3. **Connect retry logging each attempt + fail-fast** instead of silent lazy connect — the service never "pretends to be healthy" when the DB is down; clear observability for on-call.

## Next

- `pkg/httpserver` (options + graceful shutdown) + an HTTP block in `pkg/config`.
- Order REST: `POST /orders` (idempotency-key header) + `GET /orders/:id`.
- The first repo (`internal/order/repo`) — add the `squirrel` builder to `pkg/postgres` here (YAGNI: only when the repo needs it).
- ADR-0005: idempotency strategy (INSERT ... ON CONFLICT DO NOTHING).
