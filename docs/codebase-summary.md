# Codebase Summary

> Rolling document. Update whenever a new package/file is added or moved. AI agents read this first to know "where do I go look for X".

## Source layout

### `cmd/`

Service entry points. Each is a `main` package, kept minimal — just wire dependencies and call `app.Run()`.

| Path | Purpose |
|---|---|
| `cmd/order/main.go` | Order service entry |
| `cmd/inventory/main.go` | Inventory service entry |
| `cmd/payment/main.go` | Payment service entry |
| `cmd/saga/main.go` | Saga orchestrator entry |
| `cmd/notification/main.go` | Notification (Kafka consumer) entry |

### `internal/<service>/`

Service-specific code, **not importable** from outside the module (Go visibility rule).

Per-service layers (Clean Architecture — see [ADR-0002](adr/0002-clean-architecture-per-service.md)):

| Layer | Responsibility | Importing direction |
|---|---|---|
| `app/` | Composition root: wire DI, start/stop lifecycle | Imports all below + pkg |
| `entity/` | Domain types, invariants, sentinel errors | Imported by usecase |
| `usecase/` | Business logic, defines repo interfaces (consumer side) | Imports entity |
| `repo/` | Data access implementations (postgres, redis...) | Implements usecase interfaces |
| `controller/http/` | REST handlers, DTOs, trace_id middleware | Imports usecase |
| `controller/grpc/` | gRPC handlers (planned) | Imports usecase |

> `internal/order/app/` is populated: `config.go` (composed `Config` embedding the shared `pkg/config` blocks + `ORDER_HTTP_PORT` / `ORDER_DB_NAME`) and `app.go` (`Run()`: load config → logger → connect Postgres with retry → log DB version + pool stats → graceful shutdown on SIGINT/SIGTERM). It also wires repo -> usecase -> controller into pkg/httpserver and drains HTTP before closing the pool.

### `pkg/`

Cross-service shared utilities. **No business logic**. If a `pkg/X` is used by only 1 service, move it to `internal/`.

| Path | Purpose | Status |
|---|---|---|
| `pkg/logger/` | zap wrapper + trace_id context propagation | present |
| `pkg/postgres/` | pgx/v5 pool wrapper, options pattern, connect retry (exponential backoff), health/version/stat | present |
| `pkg/httpserver/` | net/http server wrapper: options pattern + graceful shutdown | present |
| `pkg/errors/` | Typed errors, error codes | when first typed error is needed |
| `pkg/config/` | Env loading (`caarlos0/env/v11`) + validation (`validator/v10`), generic `Load[T]()` + shared `App`/`Log`/`PG` blocks | present |

> `pkg/postgres` defines its own consumer-side `Logger` interface (Info/Warn/Error with `zap.Field`) so it stays decoupled from `pkg/logger`; a `logger.Logger` value satisfies it directly. Config split: **shared** reusable blocks (`App`, `Log`, `PG`) live in `pkg/config`; the **per-service composed** struct lives in that service's `app/` package (composition root).

### `proto/v1/`

gRPC `.proto` definitions. Generated `.pb.go` checked in. Planned.

### `migrations/<service>/`

`golang-migrate` SQL files: `NNNNNN_<name>.up.sql` / `NNNNNN_<name>.down.sql` (6-digit, zero-padded — matches `migrate create -seq` output and sorts lexically). Numbering starts `000001`.

### `deployments/`

| Path | Purpose |
|---|---|
| `deployments/postgres/init-databases.sh` | Creates 4 DBs on container first-start |
| `deployments/docker/` | Service-specific Dockerfiles if needed (currently none; root Dockerfile multi-target) |
| `deployments/gcp/` | Cloud Run YAML, IAM bindings (planned) |

### `docs/`

| Path | Purpose |
|---|---|
| `docs/project-overview-pdr.md` | Vision, scope, NFRs |
| `docs/system-architecture.md` | High-level architecture |
| `docs/code-standards.md` | Go conventions |
| `docs/codebase-summary.md` | This file |
| `docs/adr/` | Architecture Decision Records |
| `docs/checkpoints/` | Per-topic verify checklists |
| `docs/design/` | Per-service deep design (order-db present) |
| `docs/plans/` | Planning templates |

### `tests/`

| Path | Purpose |
|---|---|
| `tests/load/` | k6 scripts (planned) |
| `tests/chaos/` | Chaos test scripts (planned) |
| `integration-test/` | Testcontainers-based integration tests (planned) |

## Quick navigation cheatsheet

| "Where do I add X?" | Location |
|---|---|
| New REST endpoint for order | `internal/order/controller/http/` + register in `internal/order/app/` |
| New business rule for order | `internal/order/usecase/` |
| New DB query | `internal/order/repo/<engine>_<aggregate>.go` |
| New domain type | `internal/order/entity/` |
| New shared utility | `pkg/<name>/` (only if 2+ services need it) |
| New migration | `migrations/<service>/NNNNNN_<description>.up.sql` |
| New ADR | `docs/adr/NNNN-<short-title>.md` (copy from `TEMPLATE.md`) |
| New env var | `.env.example` + the parse struct in `pkg/config/` or the service's `app/config.go` |

## Inter-service dependencies (current)

```
[Order exposes REST on ORDER_HTTP_PORT and owns order_db; services do not yet talk to each other]
```

Will be updated as services start talking.

## Package import rules (enforced by review)

- `cmd/` → may import `internal/` and `pkg/`
- `internal/<service>/app/` → may import `internal/<service>/*` and `pkg/`
- `internal/<service>/usecase/` → may import `internal/<service>/entity/` only
- `internal/<service>/entity/` → may import stdlib only
- `internal/<service>/repo/` → may import `internal/<service>/entity/` and `pkg/`
- `internal/<service>/controller/` → may import `internal/<service>/usecase/` + `pkg/`
- `pkg/` → may import other `pkg/` and stdlib only. **Never** `internal/`.
- **No service depends on another service's `internal/`**. Cross-service talk goes through proto contracts.

## File-naming summary

- Code files: `kebab-case-descriptive.go`
- Test files: `<source>_test.go` (mirror)
- Migration: `NNNNNN_<description>.up.sql` / `NNNNNN_<description>.down.sql`
- ADR: `NNNN-short-title.md` (4 digits, zero-padded)
- Checkpoint: `<topic>.md`
