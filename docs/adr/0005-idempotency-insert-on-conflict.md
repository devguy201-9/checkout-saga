# ADR-0005: Idempotent order creation via INSERT ... ON CONFLICT DO NOTHING

- **Status**: Accepted
- **Deciders**: Thuan
- **Tags**: `database`, `concurrency`, `api`

## Context

`POST /orders` must be safe to retry. Networks time out, mobile clients resend,
and a load balancer may replay a request the server already handled — without
protection each retry creates another order and, downstream, another charge. The
client sends an `Idempotency-Key` header per logical attempt, and the server must
guarantee "one key -> at most one order" even when two identical requests arrive
concurrently on different instances. The `orders` table already carries a UNIQUE
`idempotency_key` column (see `docs/design/order-db.md`).

## Decision

We push the deduplication into the database: a single
`INSERT ... ON CONFLICT (idempotency_key) DO NOTHING RETURNING id`. No row
returned means the key already existed, so the repo loads and returns the order
the first request stored (`created=false` -> HTTP 200); a returned row means a
fresh order (`created=true` -> HTTP 201).

## Alternatives considered

### A. Check-then-insert in the application

```go
if existing, _ := repo.GetByIdempotencyKey(ctx, key); existing != nil { return existing }
return repo.Insert(ctx, order)
```

- Pros: obvious to read; no reliance on database-specific syntax.
- Cons: a race window between the check and the insert. Two concurrent duplicates
  both find nothing, both insert; one gets a UNIQUE violation (a 500 for a request
  that should have succeeded) — or, without the constraint, two orders exist.
- Why rejected: it is not atomic. Correctness would depend on request timing.

### B. Application-level lock (Redis SETNX / advisory lock on the key)

- Pros: works across engines; can guard multi-step flows.
- Cons: a second system in the critical path for something one index already
  guarantees; needs TTL/expiry tuning; a lock lost or expired mid-flight
  reintroduces the duplicate. More moving parts to explain and to operate.
- Why rejected: over-engineering — the invariant is already enforceable where the
  data lives.

### C. Catch the UNIQUE violation (INSERT, then handle SQLSTATE 23505)

- Pros: atomic and correct; no extra round trip on the happy path.
- Cons: uses errors for expected control flow; the repo must parse a
  driver-specific error code (`*pgconn.PgError`, `23505`), which leaks the driver
  into the error-handling path. Every failed insert also burns a sequence value
  and aborts the transaction, forcing a savepoint or retry to continue.
- Why rejected: `ON CONFLICT` expresses the same intent declaratively, and a
  no-row result is cheaper to handle than an aborted transaction.

## Consequences

### Positive

- **Race-free by construction.** The UNIQUE index serialises concurrent
  duplicates: the loser's insert conflicts, `DO NOTHING` returns no row, and it
  reads the winner's order instead. Verified by firing 100 concurrent identical
  requests and asserting exactly one row.
- **Honest status codes.** 201 vs 200 tells the client whether it created the
  order or replayed, while both are successes.
- **No extra infrastructure**, no lock lifetime to manage.

### Negative / Trade-offs

- **Postgres-specific syntax.** `ON CONFLICT` is not portable SQL.
- **Same key, different body.** The current check keys only on the key, so a
  client that reuses a key with a different payload silently gets the original
  order rather than an error.
- **Keys accumulate.** Rows are never expired, so the unique index grows forever.
- **The replay path costs an extra query** (the `SELECT` after a conflict).

### Mitigations

- Portability is a non-issue: PostgreSQL is a fixed decision for this project
  (ADR-0004), and the repo layer is the only place that would change.
- Payload mismatch: store a request fingerprint (hash of the body) alongside the
  key and return `409 Conflict` when a key is reused with different content — the
  behaviour Stripe implements. Deferred: not needed until a real client exists.
- Growth: expire keys with a TTL sweep or move them to a dedicated table once
  volume justifies it.
- The extra `SELECT` only runs on the duplicate path, which is the rare case.

## Interview defense

> "I made idempotency the database's job: `INSERT ... ON CONFLICT (idempotency_key)
> DO NOTHING RETURNING id`. Check-then-insert has a race — two concurrent retries
> both pass the check — whereas the UNIQUE index makes dedup one atomic operation,
> so the loser simply reads back the winner's order and gets 200 instead of 201.
> The trade-off is Postgres-specific syntax and that reusing a key with a different
> body returns the original order rather than a 409; I'd add a payload fingerprint
> if a real client needed that distinction."

## References

- PostgreSQL `INSERT ... ON CONFLICT` documentation.
- Stripe API: idempotent requests (key + payload fingerprint, 409 on mismatch).
- `docs/design/order-db.md` — idempotency design and the UNIQUE constraint.
