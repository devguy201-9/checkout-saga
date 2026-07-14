# Plan: Order HTTP API + Idempotent Create

Actionable spec for the next chunk. Detailed enough to hand to an agent (e.g. drive
Claude Code with it, or expand it via a spec workflow). Follows `docs/code-standards.md`,
the layering in `docs/adr/0002-clean-architecture-per-service.md`, and the schema in
`docs/design/order-db.md`.

## Context

The order service currently connects to `order_db` and has the schema, but no HTTP
surface and no repository. This chunk adds the first vertical slice: create an order
(idempotent) and fetch it by id, through all Clean Architecture layers.

## Goal

`POST /orders` creates an order and is idempotent by `Idempotency-Key`; `GET /orders/:id`
returns it. Verifiable: 100 concurrent identical `POST`s with the same key insert
exactly 1 row and all return the same order id.

## Out of scope

- Auth/JWT middleware (stub `user_id` from a header/context for now; real JWT is a later chunk).
- Inventory reservation, payment, saga wiring.
- Update/list/cancel endpoints.
- gRPC.

## Tasks

- [ ] `pkg/httpserver`: net/http server (or chi router) with functional options
      (`Port`, `ReadTimeout`, `WriteTimeout`, `ShutdownTimeout`) + `Start`/`Shutdown`
      mirroring the `pkg/postgres` options style. Add an `HTTP` block to `pkg/config`
      (`ORDER_HTTP_PORT` already in `.env.example`).
- [ ] `internal/order/entity`: `Order`, `OrderItem` structs; status constants; sentinel
      errors (`ErrOrderNotFound`, `ErrInvalidTransition`); a constructor that enforces
      invariants (total > 0, ≥1 item).
- [ ] `internal/order/usecase`: `CreateOrder(ctx, cmd)` and `GetOrder(ctx, id)`; define
      the `OrderRepo` interface here (consumer side) — `Create`, `GetByID`,
      `GetByIdempotencyKey`.
- [ ] `internal/order/repo`: pgx implementation of `OrderRepo`. `Create` uses
      `INSERT ... ON CONFLICT (idempotency_key) DO NOTHING RETURNING id`; on no-row,
      fall back to `GetByIdempotencyKey`. Parameterized queries only.
- [ ] `internal/order/controller/http`: handlers for `POST /orders` (read `Idempotency-Key`
      header; 400 if missing; 201 new / 200 existing) and `GET /orders/:id` (404 →
      `ErrOrderNotFound`). Request/response DTOs separate from entities.
- [ ] `internal/order/app`: wire repo → usecase → controller into the httpserver; start
      it in `Run`; add to graceful shutdown (drain HTTP before closing the pool).
- [ ] Tests: usecase table-tests with a fake repo; repo integration test (testcontainers
      or the running compose DB) incl. the concurrent-duplicate case.
- [ ] `docs/adr/0005-idempotency-insert-on-conflict.md`.
- [ ] Update `docs/codebase-summary.md` (repo/usecase/controller now present).
- [ ] `docs/checkpoints/order-http-idempotency.md` with sequential verify (curl happy
      path, duplicate-key returns same id, concurrent 100x → 1 row, 404 path).

## Verify checkpoint

```bash
make up && make migrate-up
make run-order &                       # or run in another terminal
# create
curl -sf -XPOST localhost:8081/orders -H 'Idempotency-Key: k1' \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"<uuid>","items":[{"product_id":"<uuid>","quantity":2,"price":10.00}]}'
# duplicate same key → same id, still one row
# concurrent: fire 100 identical POSTs with key k2, assert 1 row in orders
make test && make lint
```

## Risks / unknowns

- Router choice (stdlib `net/http` 1.22 patterns vs chi) — pick one, record in the ADR
  or the httpserver doc.
- `user_id` source before JWT exists — stub via header, mark clearly as temporary.
- Money in JSON: accept a string or number for `price`/`total`, parse to a decimal type
  carefully (don't round-trip through float).

## Notes during execution

<!-- record decisions/gotchas here while building -->

## Reflection

<!-- fill after: done / stuck / next -->
