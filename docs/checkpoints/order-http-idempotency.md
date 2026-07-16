# Order HTTP API + Idempotent Create Checkpoint

**Theme**: `pkg/httpserver` + `POST /orders` (idempotency) + `GET /orders/:id`
through the full Clean Architecture stack.

## What this covers

**Shared package**:
- `pkg/httpserver` — `net/http.Server` wrapper: functional options (`Port`,
  `ReadTimeout`, `WriteTimeout`, `ShutdownTimeout`), `Start`/`Notify`/`Shutdown`,
  Slowloris guard via `ReadHeaderTimeout`.

**Order service layers**:
- `entity/` — `Order`, `OrderItem`, `Status` constants, sentinel errors, and
  `Money` (exact minor units, never float). `NewOrder` enforces the invariants and
  computes the total from the items.
- `usecase/` — `OrderUseCase.Create` / `.GetByID`; `OrderRepo` interface declared
  here (consumer side).
- `repo/` — pgx implementation. `Create` inserts order + items in one transaction,
  idempotently (ADR-0005); items go in one `pgx.Batch`, not N round trips.
- `controller/http/` — router (stdlib Go 1.22 method+path patterns), trace_id
  middleware, DTOs, `{data|error}` envelope, handlers.
- `app/` — wires repo -> usecase -> controller into the HTTP server; drains HTTP
  before closing the pool.

**Docs**:
- `docs/adr/0005-idempotency-insert-on-conflict.md`
- `docs/codebase-summary.md` — statuses updated.

## Endpoints

| Method | Path | Success | Notes |
|---|---|---|---|
| POST | `/orders` | 201 created / 200 replay | requires `Idempotency-Key` header |
| GET | `/orders/{id}` | 200 | 404 when unknown, 400 when not a uuid |

Every response is enveloped: `{"data": ...}` or `{"error": {"code","message"}}`.
Every response echoes `X-Request-ID` (the `trace_id` in the logs).

## Verify checkpoint — sequential

Run in order; each step must pass before the next.

### 1. Build + unit tests (no DB needed)

```bash
go mod tidy          # no new dependencies were added — this should be a no-op
make fmt
make lint
make test
```

Expect: `internal/order/entity` and `internal/order/usecase` PASS.

### 2. Infra + schema

```bash
make up
make verify
make migrate-up
```

### 3. Run the service

```bash
make run-order
```

Expect, after the `postgres connected` line:
```
INFO  order service ready  {"service":"order-service","http_port":"8081"}
```

### 4. Create an order (happy path)

In a second terminal:

```bash
USER_ID=11111111-1111-1111-1111-111111111111
PRODUCT_ID=22222222-2222-2222-2222-222222222222

curl -i -XPOST localhost:8081/orders \
  -H 'Idempotency-Key: key-1' \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\",\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":2,\"price\":\"10.50\"}]}"
```

Expect `201 Created`, and a body whose `data.total_amount` is `"21.00"` (computed
server-side: 2 x 10.50), `data.status` is `PENDING`, `data.items[0].subtotal` is
`"21.00"`.

### 5. [IMPORTANT] Same key twice -> same order, no duplicate

Run the exact same curl again.

Expect `200 OK` (not 201) and the **same** `data.id`. Then:

```bash
docker exec checkout-postgres psql -U checkout -d order_db \
  -c "SELECT count(*) FROM orders WHERE idempotency_key = 'key-1';"
```

Expect `1`.

### 6. [IMPORTANT] 100 concurrent duplicates -> exactly 1 row

The race an interviewer asks about: two retries in flight at once.

```bash
for i in $(seq 1 100); do
  curl -s -o /dev/null -XPOST localhost:8081/orders \
    -H 'Idempotency-Key: race-key' \
    -H 'Content-Type: application/json' \
    -d "{\"user_id\":\"$USER_ID\",\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"price\":\"5.00\"}]}" &
done
wait

docker exec checkout-postgres psql -U checkout -d order_db \
  -c "SELECT count(*) FROM orders WHERE idempotency_key = 'race-key';"
```

Expect exactly `1`. Run it a few times — it must never be 2, and never flaky.

### 7. Read it back

```bash
ORDER_ID=$(curl -s localhost:8081/orders \
  -XPOST -H 'Idempotency-Key: key-1' -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\",\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":2,\"price\":\"10.50\"}]}" \
  | sed -E 's/.*"id":"([^"]+)".*/\1/')

curl -i localhost:8081/orders/$ORDER_ID          # 200 + items
curl -i localhost:8081/orders/33333333-3333-3333-3333-333333333333   # 404 order_not_found
curl -i localhost:8081/orders/not-a-uuid         # 400 invalid_order_id
```

### 8. Validation + money precision

```bash
# missing header -> 400 missing_idempotency_key
curl -i -XPOST localhost:8081/orders -H 'Content-Type: application/json' -d '{}'

# no items -> 400 validation_failed
curl -i -XPOST localhost:8081/orders -H 'Idempotency-Key: k-empty' \
  -H 'Content-Type: application/json' -d "{\"user_id\":\"$USER_ID\",\"items\":[]}"

# unknown field -> 400 invalid_body
curl -i -XPOST localhost:8081/orders -H 'Idempotency-Key: k-typo' \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\",\"quantiy\":1,\"items\":[]}"

# cents stay exact: 0.10 + 0.20 = 0.30, not 0.30000000000000004
curl -s -XPOST localhost:8081/orders -H 'Idempotency-Key: k-cents' \
  -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\",\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"price\":\"0.10\"},{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"price\":\"0.20\"}]}"
```

Expect the last one's `total_amount` to be exactly `"0.30"`.

### 9. Graceful shutdown

Press `Ctrl+C` in the service terminal.

```
INFO  shutdown signal received, draining http server
INFO  shutdown complete, exiting gracefully
```

Expect exit code 0, and no "connection reset" for a request that was in flight.

## Commit

```bash
git add .
git commit -m "feat(order): idempotent POST /orders + GET /orders/:id

- pkg/httpserver: net/http wrapper, functional options, graceful shutdown
- entity: Order/OrderItem/Status, Money as exact minor units (no float),
  NewOrder enforces invariants and computes the total server-side
- usecase: Create/GetByID, OrderRepo interface on the consumer side
- repo: pgx impl, INSERT ON CONFLICT DO NOTHING RETURNING id + read-back
  of the winner on duplicate key, items inserted in one batch
- controller/http: stdlib 1.22 routing, trace_id middleware, {data|error}
  envelope, sentinel-to-status mapping
- app: wire repo -> usecase -> controller, drain HTTP before closing the pool
- docs(adr): 0005 idempotency via INSERT ON CONFLICT"
git tag v0.3.0
git push && git push --tags
```

## Common pitfalls

- **`404 page not found` on every route**: the stdlib method+path patterns
  ("POST /orders") need Go 1.22+. Check `go version`.
- **`400 invalid_body` on a seemingly fine payload**: unknown fields are rejected
  on purpose — check for a typo in a field name.
- **`price` sent as a bare float in a client**: JSON numbers are accepted and
  parsed exactly, but any client-side float arithmetic before sending can still
  drift. Send money as a string.
- **`500 internal_error` on create**: read the log line with that request's
  `trace_id` — the detail is logged, not returned.
- **Port already in use**: the service now fails loudly via `server.Notify()`
  instead of hanging.

## Trade-offs chosen

1. **Stdlib `net/http` routing instead of chi/gin** — Go 1.22 method+path patterns
   cover these routes, so no router dependency to justify. If middleware chains or
   route groups grow, revisit and record an ADR.
2. **Money as int64 minor units instead of a decimal library** — exact, zero
   dependencies, and it maps directly to `DECIMAL(10,2)`. Money crosses the DB
   boundary as text (`$2::numeric`, `total_amount::text`) so nothing rounds. A
   decimal library would be warranted for FX or fractional-cent pricing.
3. **Idempotency in the DB, not the app** — ADR-0005. `ON CONFLICT` is atomic;
   check-then-insert has a race window.
4. **The repo returns `created bool`** rather than a sentinel "already exists"
   error: a replay is a success, and a sentinel would push HTTP semantics into the
   domain.
5. **`user_id` still comes from the body** — a temporary stub. It moves to the
   request context once JWT middleware exists; until then a client could claim any
   user, which is fine for a local checkpoint and unacceptable in production.

## Next

- JWT middleware: authenticate, put `user_id` in the context, drop it from the body.
- Integration tests with testcontainers (real Postgres) for the repo, including
  the concurrent-duplicate case now verified by hand above.
- Inventory service: reservation with `SELECT ... FOR UPDATE` (no oversell).
