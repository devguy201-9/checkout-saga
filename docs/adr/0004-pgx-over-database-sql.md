# ADR-0004: Use pgx/v5 (pgxpool) instead of database/sql

- **Status**: Accepted
- **Deciders**: Thuan
- **Related**: ADR-0002 (Clean Architecture), `pkg/postgres`, `docs/code-standards.md`

## Context

The order service (and later inventory/payment/saga) needs a PostgreSQL driver for:

- A controlled connection pool (max/min conns, lifetime, idle).
- Explicit transactions for pessimistic locking (`FOR UPDATE`) and the outbox pattern.
- Good performance under high concurrency (the no-oversell 100-goroutine test that comes later).
- An idiomatic Go API with `context.Context` on every call.

The driver choice affects the entire repo layer, so it must be settled early and recorded.

## Decision

Use **`github.com/jackc/pgx/v5`** in native mode (`pgxpool.Pool`), NOT through `database/sql`.

`pkg/postgres` wraps `pgxpool` with: an options pattern, connect retry (exponential
backoff, logging each attempt), a health check, and version/stat reads.

SQL is hand-written (raw for static queries, `squirrel` for dynamic ones — added
when the first repo needs it), always parameterized (`$1, $2`), never string-formatted.

## Alternatives considered

| Option | Why rejected |
|---|---|
| `database/sql` + `lib/pq` | `lib/pq` is in maintenance mode (its author recommends pgx). Missing many native Postgres types. |
| `database/sql` + `pgx` stdlib adapter | Usable, but loses the native pgx API (batch, `CopyFrom`, strong type mapping) and adds an abstraction layer this single-owner monorepo does not need. |
| `sqlx` | Convenient struct scanning, but still sits on `database/sql` → same limitations; native pgx is already ergonomic enough. |
| GORM | Already excluded in CLAUDE.md/code-standards: hides SQL → harder to learn and harder to defend in an interview; generates queries that are hard to control; works against the "understand deeply" goal. |

## Consequences

**Positive**

- Native API: `QueryRow/Query/Exec/Begin` take `ctx`; batch + `CopyFrom` when needed.
- `pgxpool` manages the pool directly and exposes `Stat()` for metrics/observability.
- Strong Postgres type mapping (uuid, numeric, jsonb, tstzrange) — useful for the saga JSONB payload and inventory.
- The driver is actively maintained and high-performance.

**Negative / trade-offs**

- Different from the `database/sql` API → cannot plug straight into libraries that expect `*sql.DB` (e.g. some test helpers). Accepted: tests use a real pool via testcontainers (added later).
- Some tooling (e.g. `golang-migrate`) runs as a separate CLI rather than sharing the connection — accepted, migrations are a separate operational task.

## Interview defense (one sentence)

> "I chose native pgx/v5 because I need explicit transactions for `FOR UPDATE` and the outbox, good Postgres type mapping for the JSONB saga payload, and `pgxpool.Stat()` for metrics. With `database/sql`/`lib/pq`, `lib/pq` is already in maintenance mode; and GORM hides SQL, so I deliberately avoid it to keep control of and understanding of my queries."
