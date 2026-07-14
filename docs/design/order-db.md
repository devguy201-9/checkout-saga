# Design Deep-Dive: order_db + Connection Layer

Reference for the `orders` / `order_items` schema and the `pkg/postgres` connection
pool. Explains **why** each decision was made, the trade-offs, the failure modes,
and the questions an interviewer is likely to ask. Pair this with
`migrations/order/000001_orders.up.sql`, `pkg/postgres/`, and `pkg/config/`.

---

## 1. Scope

The order service owns `order_db` exclusively — no other service reads or writes
it (database-per-service). Cross-service data flows through APIs/events, never
through a shared schema. This keeps the order service's storage a private
implementation detail it can change without coordinating with others.

This layer delivers: the schema, the pooled connection with startup retry, and
config plumbing. It does **not** yet include repositories, HTTP handlers, or the
saga — those consume this foundation.

---

## 2. Schema: `orders`

```
id              UUID  PK  DEFAULT gen_random_uuid()
user_id         UUID  NOT NULL
total_amount    DECIMAL(10,2) NOT NULL CHECK (> 0)
status          VARCHAR(30) NOT NULL DEFAULT 'PENDING' CHECK (status IN (...))
idempotency_key VARCHAR(255) NOT NULL UNIQUE
version         INT NOT NULL DEFAULT 0
created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
```

Column-by-column rationale:

- **`id UUID` (surrogate key, `gen_random_uuid()`)** — UUID over auto-increment
  `BIGINT` because order IDs are exposed to clients and cross service boundaries
  (saga passes an order ID to inventory/payment). A sequential integer leaks
  volume ("how many orders exist") and invites enumeration; a UUID does not.
  `gen_random_uuid()` is built into PostgreSQL 13+, so no `pgcrypto` extension is
  needed. Trade-off: UUID v4 is random, so index inserts are not append-only and
  the primary-key B-tree fragments more than a monotonic key would. Acceptable at
  this scale; if write throughput ever became the bottleneck, UUID v7
  (time-ordered) would restore locality without giving up global uniqueness.

- **`user_id UUID`** — the owner. Not a foreign key: users live in a different
  bounded context (auth), so there is no `users` table here to reference. Integrity
  is enforced at the application boundary (a valid JWT), not by the DB.

- **`total_amount DECIMAL(10,2)`** — money is **never** `FLOAT`/`DOUBLE`.
  Binary floating point cannot represent values like `0.10` exactly, so sums drift
  by cents. `DECIMAL(10,2)` is exact base-10 with 2 fractional digits — up to
  99,999,999.99, comfortably beyond a single order. `CHECK (> 0)`: an order with a
  zero or negative total is a bug, rejected at the DB as a last line of defense.

- **`status VARCHAR(30) + CHECK`** — the order-lifecycle state (see §3). VARCHAR +
  CHECK instead of a native `ENUM` type. Postgres ENUMs are awkward to evolve:
  adding a value needs `ALTER TYPE ... ADD VALUE` (which historically could not run
  inside a transaction), and reordering/removing values is worse. A CHECK
  constraint is edited by a normal migration (`DROP CONSTRAINT` + `ADD CONSTRAINT`)
  and is still strict enough to reject junk. Trade-off: slightly larger row than a
  4-byte enum OID — negligible.

- **`idempotency_key VARCHAR(255) UNIQUE`** — the client-supplied dedup key (see
  §4). The `UNIQUE` constraint is the whole mechanism: the database, not the app,
  guarantees "one key → one order" even under concurrent duplicate requests.

- **`version INT DEFAULT 0`** — optimistic-lock counter (see §5). Incremented by
  the app inside each `UPDATE`.

- **`created_at` / `updated_at TIMESTAMPTZ`** — `TIMESTAMPTZ`, never naive
  `TIMESTAMP`: it stores an absolute instant (UTC) and is unambiguous across server
  time zones and DST. There is deliberately **no** `updated_at` trigger — see §5
  for why the app owns that column.

---

## 3. Order status state machine

```
                 ┌───────────────────────┐
   PENDING ──────► INVENTORY_RESERVED ────► PAYMENT_PROCESSING ──► COMPLETED
      │                    │                        │
      │                    │                        └──► PAYMENT_FAILED
      │                    └──► (reserve fails) ─────────► INVENTORY_INSUFFICIENT
      └──► CANCELLED (user / timeout / compensation)
```

The CHECK constraint enumerates exactly these seven states, so the DB rejects any
value the code does not intend. **Transition** validity (e.g. you cannot go
`COMPLETED → PENDING`) is enforced in the domain layer (`entity`/`usecase`), not by
the DB — a CHECK constraint can validate a single row's value but not "what the
previous value was". The saga is the component that drives these transitions and
issues compensations; the terminal failure states (`PAYMENT_FAILED`,
`INVENTORY_INSUFFICIENT`, `CANCELLED`) are what compensation writes.

---

## 4. Idempotency design

**Problem:** networks retry. A client that times out on `POST /orders` will resend
the same request; without protection you create duplicate orders and double-charge.

**Mechanism:** the client sends a unique `Idempotency-Key` (typically a UUID) per
logical order attempt. The column is `UNIQUE`. The create path is an atomic upsert:

```sql
INSERT INTO orders (user_id, total_amount, status, idempotency_key)
VALUES ($1, $2, 'PENDING', $3)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING id;
```

- First request: row inserts, `RETURNING id` yields the new order → `201 Created`.
- Duplicate request: `ON CONFLICT DO NOTHING` inserts nothing, `RETURNING` yields
  no row → the handler looks the existing order up by key and returns it (`200 OK`),
  same result, no second row, no second charge.

Why push this to the database rather than "check then insert" in the app? Because a
check-then-insert has a race: two concurrent duplicates both pass the check, both
insert. The `UNIQUE` index makes the dedup a single atomic operation the database
serializes for us. This is the reasoning captured for ADR-0005 in the next chunk.

Bounded growth note: idempotency keys accumulate. In production you would expire
them (a TTL sweep or a separate `idempotency_keys` table with `created_at`), but the
UNIQUE-on-orders approach is the simplest correct starting point.

---

## 5. Concurrency & locking

Two different strategies, chosen per aggregate by conflict probability:

**Optimistic (here, on `orders`).** Orders have low write contention — usually one
actor mutates a given order at a time (the saga stepping it forward). Optimistic
locking assumes no conflict and verifies at write time:

```sql
UPDATE orders
   SET status = $2, version = version + 1, updated_at = NOW()
 WHERE id = $1 AND version = $expected;
-- rows affected = 0  →  someone else moved it first → reload & retry / abort
```

If `version` no longer matches, the `UPDATE` touches 0 rows and the app knows it
raced. No row locks are held across the read→modify→write gap, so readers and other
orders are never blocked. This is why there is **no `updated_at` trigger**: the app
already sets `updated_at` inside this same `UPDATE`, and a `BEFORE UPDATE` trigger
would fight the app for that column and muddy the optimistic-lock write.

**Pessimistic (later, in inventory).** Stock decrement is the opposite: high
contention on a hot row (everyone buys the popular SKU) and oversell must be
impossible. There the inventory service will take a row lock —
`SELECT ... FOR UPDATE` — so concurrent buyers serialize on that row and the
"available >= requested" check is authoritative. Optimistic retries under heavy
contention would thrash; a lock is correct there. Different tool for a different
contention profile, which is itself a defensible design answer.

---

## 6. Indexing strategy

| Index | Column(s) | Query it serves | Notes |
|---|---|---|---|
| PK | `id` | point lookup `WHERE id = $1` (`GET /orders/:id`) | Unique B-tree, always present |
| `orders_idempotency_key_key` | `idempotency_key` | the `ON CONFLICT` upsert + dup lookup | Created implicitly by `UNIQUE` |
| `idx_orders_user_id` | `user_id` | "list my orders" `WHERE user_id = $1` | Medium selectivity |
| `idx_orders_status` | `status` | ops/saga scans `WHERE status = 'PENDING'` | Low selectivity — see below |
| `idx_order_items_order_id` | `order_id` | load items for an order (FK join) | The classic "index your FKs" |

Reasoning and trade-offs:

- **Every index is a write tax.** Each `INSERT`/`UPDATE` must maintain every index,
  so indexes are added for queries that actually run, not speculatively.
- **`idx_order_items_order_id`** — Postgres does **not** auto-create an index on the
  referencing side of a foreign key. Without it, "get all items for this order" is a
  sequential scan, and cascading deletes get slow. Indexing FK columns is a standard
  fix.
- **`idx_orders_status` is low-selectivity** (only ~7 distinct values). A plain B-tree
  on a low-cardinality column helps when one value is rare (e.g. few `PENDING` rows
  among millions COMPLETED) but the planner may ignore it when the value is common.
  If the real access pattern is "find stuck PENDING orders", a **partial index**
  `WHERE status = 'PENDING'` would be smaller and strictly better — a concrete
  optimization to make when the query pattern is confirmed rather than guessed.
- **Composite index candidate:** if "list a user's orders filtered by status" becomes
  common, `(user_id, status)` (in that order — equality column first) would serve it
  and also cover `WHERE user_id = $1` alone, making the single-column `idx_orders_user_id`
  redundant. Not added yet: YAGNI until the query exists.
- **Deliberately not indexed:** `total_amount`, `created_at`, `version`. No query
  filters or sorts on them yet; indexing them now would only slow writes.

---

## 7. Connection pool design (`pkg/postgres`)

Backed by `pgxpool`. Defaults (overridable via functional options and
`POSTGRES_POOL_*` env):

| Setting | Default | Why |
|---|---|---|
| `MaxConns` | 25 | Ceiling of concurrent DB connections this instance holds |
| `MinConns` | 5 | Warm idle connections so the first requests don't pay connect latency |
| `ConnAttempts` | 5 | Startup retries before fail-fast |
| `ConnTimeout` | 5s | Per-Ping timeout — connect never hangs unbounded |
| `RetryBackoff` | 1s (doubles) | 1s, 2s, 4s, 8s — backs off a struggling DB |
| `MaxConnLifetime` | 1h | Recycle connections so load rebalances, memory doesn't creep |
| `MaxConnIdleTime` | 30m | Release rarely-used connections back down toward MinConns |

**Sizing math — the important part.** More connections is not more throughput. A
connection is only doing work while a CPU core (or a disk) serves its query; beyond
that, extra connections just queue and add context-switch overhead. The well-known
starting heuristic is roughly `connections ≈ (cpu_cores × 2) + effective_spindles`,
which for a small managed Postgres lands in the low tens — hence 25, not 200.

The **global** constraint matters more here: PostgreSQL's `max_connections` defaults
to ~100, and every backend costs memory. With 5 services this repo could in theory
open `5 × 25 = 125` connections and exhaust the server. Mitigations, in order of
reach: (1) tune each service's `POSTGRES_POOL_MAX` to its real concurrency, since
not every service is DB-heavy or scaled equally; (2) put **PgBouncer** in front in
transaction-pooling mode so hundreds of app-side "connections" multiplex onto a small
set of real backends. The single-owner dev setup here doesn't hit the ceiling, but
naming the ceiling and its fix is the senior answer.

**`MinConns > MaxConns` guard.** `pgxpool` rejects a config where MinConns exceeds
MaxConns. If someone sets a tiny `POSTGRES_POOL_MAX`, the wrapper clamps MinConns
down to MaxConns rather than crashing — defensive, and covered by a unit test.

**Retry + fail-fast rationale.** On startup the pool Pings with retry
(1s→2s→4s→8s), logging every attempt. If all attempts fail, `New` returns a wrapped
error and the service exits non-zero. This is deliberate: a service that "starts"
but can't reach its database is worse than one that fails loudly — an orchestrator
(Cloud Run / k8s) will restart a crashed instance, whereas a silently-degraded one
serves errors and passes naive health checks. Logging each attempt makes the "DB was
down for N seconds at boot" story visible to on-call. The retry uses `zap` fields
(structured), so it is queryable in log aggregation, not a formatted string.

---

## 8. Failure modes

| Scenario | Behavior | Where |
|---|---|---|
| DB down at startup | 5 retries w/ backoff, log each, then exit 1 | `pkg/postgres.New` |
| Missing/invalid env (e.g. no `POSTGRES_PASSWORD`) | `config.Load` validation error → fail-fast, no partial boot | `pkg/config` |
| DB drops mid-flight | query returns error; caller maps to 5xx; pool re-establishes on next use | repo layer (later) |
| Duplicate `POST /orders` | `ON CONFLICT DO NOTHING` → existing order returned, no dup | repo/usecase (later) |
| Concurrent status update race | optimistic `WHERE version = $expected` → 0 rows → reload/abort | repo/usecase (later) |
| Migration fails midway | `schema_migrations.dirty = true`; `migrate force` + fix | golang-migrate |

---

## 9. Migration strategy

`golang-migrate`, files `NNNNNN_<name>.{up,down}.sql`, 6-digit sequential. `up`
applies forward; `down` is written FK-safe (drop `order_items` before `orders`) and
is used mainly for local iteration. In production, migrations are **forward-only**:
you never `down` a released migration against real data; you write a new corrective
`up`. The CLI is installed separately (`go install .../migrate`) and is not a module
dependency, so the service binary stays lean.

---

## 10. Interview Q&A (defend the design)

- **Why UUID, not auto-increment?** Exposed across service boundaries; avoids
  enumeration/volume leakage. Cost: random-insert index fragmentation, solvable with
  UUID v7 if needed.
- **Why DECIMAL for money?** Exactness. Floats drift on values like 0.10.
- **Why VARCHAR+CHECK, not ENUM?** Cheap to evolve via a migration; ENUM changes are
  awkward and historically non-transactional.
- **Optimistic vs pessimistic — how do you choose?** By conflict probability. Low
  contention (orders) → optimistic `version`; hot contested rows with a hard
  invariant (inventory stock) → pessimistic `FOR UPDATE`.
- **How is idempotency race-free?** `UNIQUE` + `INSERT ON CONFLICT` makes dedup a
  single atomic DB operation, unlike check-then-insert.
- **How do you size the pool?** Not "bigger is better" — bounded by cores and by
  server `max_connections`; PgBouncer when app instances multiply.
- **What happens when the DB is down at boot?** Bounded retry with backoff, then
  fail-fast (exit non-zero) so the orchestrator restarts, plus structured logs.
- **Why no `updated_at` trigger?** The optimistic-lock `UPDATE` sets `updated_at`
  itself; a trigger would fight it.

---

## 11. Future considerations (when the need is real)

- **Idempotency-key expiry** — TTL sweep or a dedicated table once keys accumulate.
- **Partial index** `WHERE status = 'PENDING'` once the "stuck orders" query is confirmed.
- **Outbox table** in `order_db` for the Kafka event-publishing chunk (transactional outbox).
- **UUID v7** for primary keys if write locality becomes a bottleneck.
- **Read replica / PgBouncer** if read load or connection count grows.
