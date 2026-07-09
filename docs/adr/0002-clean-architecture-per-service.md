# ADR-0002: Clean Architecture per service

- **Status**: Accepted
- **Deciders**: Thuan
- **Tags**: `architecture`, `code-structure`

## Context

Each service has an HTTP handler, business logic, DB access, and a gRPC handler
(added later). We need to decide how to layer the code so it is:

- Testable (easy to mock the DB)
- Maintainable (swapping the DB does not require rewriting the handler)
- Clear about the unit-of-work boundary
- Defensible in a senior-level interview

## Decision

**Clean Architecture, 4 layers** per service: `entity` → `usecase` → `repo` & `controller`.

Dependency direction:
```
controller ─► usecase ─► entity
                 ▲
                 │ (interface)
              repo ◄── repo/impl
```

The repo **interface** lives in `usecase/contracts.go` (consumer side). The repo
**implementation** lives in `repo/<engine>_<name>.go` (e.g. `repo/order_postgres.go`).

## Alternatives considered

### A. Flat structure (handler + db in the same package)

- Pros: minimal indirection, fast to write small features.
- Cons: tests need a real DB or heavy refactoring. Business logic leaks into the HTTP layer (status codes mixed with rules). Hard to scale beyond a handful of endpoints.
- Why rejected: fine for prototypes, not for demonstrating senior-grade engineering.

### B. Hexagonal Architecture (ports & adapters)

- Pros: emphasizes the ports/adapters duality; popular in the Java/Spring world.
- Cons: conceptually the same as Clean Architecture with different naming.
- Why rejected: Clean Architecture has more widely shared vocabulary in the Go community, and the structural reference (go-clean-template) already uses it.

### C. DDD with aggregates + bounded contexts

- Pros: a rich domain model, with invariants enforced at the aggregate root.
- Cons: a senior interviewer will dig deep (e.g. "when does an aggregate boundary span two entities?"), and aggregate-root invariants are an area I do not yet command well.
- Why rejected: defensibility concern. Clean Architecture delivers most of the value without the terminology load.

## Consequences

### Positive

- Each layer is independently testable: the `usecase` is tested with a mock repo, no PostgreSQL needed.
- Swapping DB technology = rewriting the repo only; usecase/handler unchanged.
- The repo interface in the usecase means **the caller defines what it needs** (the Go idiom).
- Clear "where do I add this feature": validation → handler, rule → usecase, query → repo.

### Negative / Trade-offs

- More files/packages per service. A single-endpoint feature may touch 4 files.
- Risk of an "anemic domain" (entities = struct + no behavior). Invariants must be deliberately placed on the entity.
- Mapping between entity ↔ DB row ↔ HTTP DTO can feel boilerplate-heavy.

### Mitigations

- Keep entities rich: validation methods, state-transition methods.
- Use codegen for DTO mapping if it becomes repetitive (e.g. `oapi-codegen` for OpenAPI).
- The file-size rule (< 200 LOC) keeps each layer's file small enough to navigate.

## What we explicitly DON'T do

- **No DDD aggregate-root vocabulary** in code or interview talk (defensibility).
- **No bounded-context terminology** — the service boundary suffices.
- **No CQRS** — a shared read-write model is fine.
- **No event sourcing** — out of scope.

## Interview defense

> "I chose Clean Architecture per service for 4 clear layers: entity holds domain logic, usecase orchestrates, repo abstracts data access, controller exposes I/O. Testability is highest: the usecase is tested with a mock repo. The trade-off is more boilerplate than a flat structure — I accept it because a demo project needs defensibility, not the shortest line count. I do not use DDD aggregate roots because the scope does not yet need a rich invariant network, and I will not introduce a concept I cannot fully defend."

## References

- [Uncle Bob — Clean Architecture](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [go-clean-template](https://github.com/evrone/go-clean-template) — structural reference
