# Foundation Checkpoint

**Theme**: Foundation infrastructure + 5 service skeletons

## What this covers

**Infrastructure**:
- Docker Compose: PostgreSQL 16 + Redis 7 + Adminer, with healthchecks + resource limits
- Init script creates 4 isolated databases: `order_db`, `inventory_db`, `payment_db`, `saga_db`
- Multi-target distroless Dockerfile (pick the service via `--build-arg SERVICE=`)

**Code**:
- 5 service skeletons (`cmd/<service>/main.go`) — version + commit ldflags, graceful shutdown via `signal.NotifyContext`
- `pkg/logger` with zap, `trace_id` propagation via `context.Context`, JSON (prod) / console (dev) encoders
- `pkg/logger` unit tests (table-driven, parallel)

**Quality gates (CI)**:
- `.golangci.yml` v2 strict (cyclop, gocognit, gosec, errorlint, ...)
- `.github/workflows/ci.yml`: lint + race + vuln + build matrix
- `make pre-commit` runs all gates locally

**Documentation**:
- `CLAUDE.md` working agreement (also serves as Claude Project Instructions)
- `docs/project-overview-pdr.md` — PDR with success criteria + NFRs
- `docs/system-architecture.md` — high-level + state machines + failure modes
- `docs/code-standards.md` — Go conventions, naming, error handling, testing
- `docs/codebase-summary.md` — rolling "where things live" doc
- `docs/adr/0001-monorepo-single-go-module.md` — repo structure decision
- `docs/adr/0002-clean-architecture-per-service.md` — layering decision
- `docs/adr/0003-zap-with-tracing-context.md` — logging decision
- `docs/adr/TEMPLATE.md` — template for future ADRs
- `docs/plans/checkpoint-template.md` — planning template

## Verify checkpoint — sequential

Run in order; each step must pass before the next.

### 1. Setup env

```bash
cp .env.example .env
go mod tidy
```

Expect: `.env` exists, `go.sum` populated, no errors.

### 2. Start infra

```bash
make up
```

Expect:
```
NAME                 STATUS                   PORTS
checkout-adminer     Up X seconds             0.0.0.0:8090->8080/tcp
checkout-postgres    Up X seconds (healthy)   0.0.0.0:5432->5432/tcp
checkout-redis       Up X seconds (healthy)   0.0.0.0:6379->6379/tcp
```

If UNHEALTHY → `docker compose logs postgres redis` to debug.

### 3. Verify infra

```bash
make verify
```

Expect output:
```
✓ 4 databases exist
PONG
✓ Infra OK
```

If a DB is missing (init script did not run): `make down-v && make up`.

### 4. Build all services

```bash
make build
```

Expect:
```
Building order ...
Building inventory ...
Building payment ...
Building saga ...
Building notification ...
✓ All services built
```

5 binaries in `bin/`. Check:
```bash
ls -la bin/
file bin/order   # should be: ELF 64-bit LSB executable
```

### 5. Smoke test one service

```bash
make run-order
```

Expect log (console in dev mode):
```
INFO  service starting  {"service":"order-service","version":"dev","commit":"unknown","go_version":"go1.22.X","port":"8081"}
```

Press `Ctrl+C`:
```
INFO  shutdown signal received, exiting gracefully
```

Repeat for `make run-inventory`, `run-payment`, `run-saga`, `run-notification`.

### 6. Quality gates

```bash
make fmt       # apply formatter
make tidy      # tidy go.mod
make lint      # golangci-lint strict
make test      # race + coverage
make vuln      # govulncheck
```

Each must pass. If `make lint` reports issues, fix them per each linter's recommendation.

## Commit

```bash
git init
git add .
git commit -m "feat(foundation): senior-grade scaffold

- Monorepo single go.mod, 5 service skeletons (cmd/*/main.go)
- Docker Compose: PostgreSQL 16 + Redis 7 + Adminer with healthchecks
- pkg/logger: zap + trace_id context propagation (ADR-0003)
- Strict golangci-lint v2 config + CI: lint/race/vuln/build matrix
- Multi-target distroless Dockerfile
- Docs scaffold: PDR, system-architecture, code-standards, codebase-summary
- ADRs: 0001 monorepo, 0002 clean architecture, 0003 zap logging"

git remote add origin https://github.com/devguy201-9/checkout-saga.git
git branch -M main
git push -u origin main
```

## Common pitfalls

- **Container UNHEALTHY**: port already in use → `lsof -i :5432` or `:6379` to find the old process.
- **Init script did not run**: the volume already has data. `make down-v` to reset.
- **`make lint` fails on empty packages**: a `.gitkeep` with no go file → fine, golangci-lint skips empty dirs.
- **`make vuln` warning**: normal if there are only stdlib deps. Govulncheck checks transitive deps.
- **Adminer login fails**: the Server field must be `postgres` (the container name), not `localhost`.

## Trade-offs chosen

1. **Single-module monorepo** instead of workspaces — ADR-0001. Defensible: simpler for a single owner, no need for independent versioning.
2. **Clean Architecture** instead of flat or DDD — ADR-0002. Defensible: testability + clear boundaries, avoids DDD vocabulary not yet mastered.
3. **zap instead of slog** — ADR-0003. Defensible: production maturity + OpenTelemetry integration; slog is the future direction but zap is still the convention.
4. **Distroless Dockerfile** from the start rather than later. Defensible: security best practice, ~10MB image, no shell attack surface.
5. **CI from the start** rather than later. Defensible: prevent rot — broken code never reaches main. Senior-grade discipline.

## Next

- `golang-migrate` setup for `order_db`
- `orders` + `order_items` schema per `docs/system-architecture.md`
- `pkg/postgres` connection pool with `pgx/v5`, retry on startup
- `cmd/order` connects to the DB, logs version + pool stats
- Makefile: `migrate-up`, `migrate-down`, `migrate-create`
- ADR-0004: `pgx/v5` over `database/sql`
- `docs/checkpoints/postgres-connection.md`
