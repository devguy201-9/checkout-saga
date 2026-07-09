# Code Standards

## Language & version

- Go 1.22+ only. No build tags / fallbacks for older versions.
- Module path: `github.com/devguy201-9/checkout-saga`.

## Package structure (Clean Architecture)

Per service in `internal/<service>/`:

```
internal/order/
├── app/             # Composition root — wire DI, lifecycle
│   └── app.go
├── entity/          # Domain entities + invariants
│   ├── order.go
│   └── errors.go
├── usecase/         # Business logic — orchestrate entities
│   ├── contracts.go # Interfaces (consumer-side)
│   └── order_usecase.go
├── repo/            # Data access — implement repo interfaces
│   └── order_postgres.go
└── controller/
    ├── http/        # REST handlers
    └── grpc/        # gRPC handlers
```

**Dependency direction**: `controller → usecase → entity` and `usecase → repo (interface) ← repo/impl`. The repo interface lives **in the usecase package** (consumer side); the implementation lives in the repo package.

### Why this layering

ADR `0002-clean-architecture-per-service.md` has the detail. In short: testability (easy to mock the repo), preservation of business logic (no SQL leaking into the handler), and a clear unit-of-work boundary.

## Naming

| Item | Convention |
|---|---|
| Files | `kebab-case-descriptive.go` (e.g. `order-idempotency.go`) |
| Packages | one word, lowercase, no underscore |
| Exported types | `PascalCase` |
| Unexported | `camelCase` |
| Interfaces | `<Noun>Repo`, `<Noun>UseCase` (not `IRepo`) |
| Constants | `PascalCase` or `camelCase`; group unexported package-level with a `_` prefix (e.g. `_defaultTimeout`) |
| Acronyms | uppercase consistently: `userID`, `orderID`, not `userId` |
| Receiver names | 1-2 letters, consistent across methods (e.g. `o *Order`, `r *orderRepo`) |
| Test files | `*_test.go`, table-driven with `t.Run(name, ...)` |

## Errors

### Wrap, don't lose

```go
if err := db.QueryRow(...).Scan(&x); err != nil {
    return fmt.Errorf("orderRepo.GetByID: %w", err)
}
```

Format: `<scope>.<action>: %w`. Scope = receiver type or package.subject. Action = what failed.

### Sentinel errors

Domain-level expected errors in `entity/errors.go`:

```go
var (
    ErrOrderNotFound        = errors.New("order: not found")
    ErrIdempotencyConflict  = errors.New("order: idempotency key in use")
    ErrInsufficientStock    = errors.New("inventory: insufficient stock")
)
```

Callers use `errors.Is(err, entity.ErrOrderNotFound)`.

### Typed errors (when status code matters)

```go
type ValidationError struct {
    Field   string
    Message string
}
func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }
```

Callers use `errors.As(err, &valErr)`.

## Context

- `ctx context.Context` is the **first parameter** of any function that calls DB / HTTP / gRPC / Redis.
- Never store `ctx` in a struct.
- Never pass a `nil` ctx — use `context.TODO()` if there is truly nothing else.
- Always check `ctx.Done()` in loops longer than a few iterations.

## Logging

- `pkg/logger` is the only logger entry point. No `log.Println` allowed.
- Every log line in a request scope must carry `trace_id`.
- Levels:
  - `Debug`: dev troubleshooting, off in prod
  - `Info`: meaningful business events (order created, payment succeeded)
  - `Warn`: recoverable anomaly (retry, fallback used)
  - `Error`: the caller will see an error response, or the operation failed
  - `Fatal`: the process cannot continue (init failure only)
- Fields: structured, not interpolated. `logger.Info("order created", zap.String("order_id", id))`, not `Info(fmt.Sprintf(...))`.

## SQL

- Use `pgx/v5` directly. Avoid ORMs.
- Query construction: the `squirrel` builder when dynamic, a raw string when static.
- Always parameterize (`$1`, `$2`). Never string-format SQL values.
- `SELECT * FROM` is forbidden. Always list columns.
- Transactions: `defer tx.Rollback()` immediately after `BeginTx`.
- Locks: document WHY pessimistic vs optimistic in a code comment + ADR.

```go
// Pessimistic lock: oversell is a correctness issue and the conflict rate is high.
row := tx.QueryRow(ctx, `SELECT available_stock FROM products WHERE id = $1 FOR UPDATE`, productID)
```

## Concurrency

- A goroutine has a clear lifecycle. Never `go fn()` without a way to wait / cancel.
- `sync.WaitGroup` or `errgroup.Group` for known fan-out.
- `select` with `<-ctx.Done()` in any long-running goroutine.
- Channels: prefer `chan T` (not `chan interface{}`). Direction in signatures: `chan<- T`, `<-chan T`.
- Mutex: small critical section. Prefer immutable data + channel hand-off.
- No goroutine leaks: every test with goroutines runs `goleak.VerifyNone(t)`.

## Testing

- Table-driven, names describe behavior:
  ```go
  func TestReserve_RejectsWhenStockInsufficient(t *testing.T) { ... }
  ```
- Use `testify/require` (fail-fast) for setup, `testify/assert` for non-critical checks. (Optional — the `testing` stdlib is also fine if preferred.)
- Mock interfaces with `gomock` (`mockgen` from the `Makefile`).
- Race detector ALWAYS on in CI (`go test -race`).
- No `time.Sleep` in tests > 10ms. Use channels / `eventually` patterns.
- Integration tests: `_test.go` files in the `integration-test/` package, gated by the build tag `//go:build integration`.

## File size & complexity

- File > 200 LOC: split it. Common split axes:
  - Per entity (`order.go`, `order_item.go`)
  - Per concern (`order_validation.go`, `order_serialization.go`)
- Function > 80 LOC or cyclomatic complexity > 15: refactor.
- Cyclomatic and cognitive complexity are enforced by `.golangci.yml`.

## Forbidden patterns

- `init()` functions outside the `main` package
- Global mutable state (config, DB, etc.) — pass via DI
- `panic` outside `main.go` init phase
- `log.Fatal` outside `main.go`
- `interface{}` (use `any` if needed, but rarely)
- Empty interfaces in business code
- Generic "utils" or "common" packages — give it a specific name

## Conventional Commits

```
<type>(<scope>): <subject>

[optional body]

[optional footer]
```

Types: `feat | fix | refactor | test | docs | chore | perf | ci | build`
Scopes: `order | inventory | payment | saga | notification | infra | deps | docs`

Examples:
```
feat(order): add idempotency key handling
fix(inventory): close rows in Reserve to prevent connection leak
refactor(saga): extract state transition into separate function
docs(adr): add 0005 pessimistic lock rationale
```

## Code review checklist (self-review before commit)

- [ ] All `context.Context` propagated
- [ ] All errors wrapped with scope
- [ ] No `log.X` outside `main`
- [ ] Tests added/updated
- [ ] `make lint` passes
- [ ] `make test` passes with race
- [ ] No commented-out code
- [ ] No `TODO` without a ticket/note
- [ ] ADR if a new significant decision

## When to write an ADR

Write an ADR when:

- Picking between multiple viable solutions
- Introducing a new pattern / library
- Departing from project conventions
- A trade-off that could be challenged in an interview

ADRs go in `docs/adr/NNNN-short-title.md`. Template: `docs/adr/TEMPLATE.md`.
