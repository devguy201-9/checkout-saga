# CLAUDE.md — Working Agreement

This file defines how Claude (and any AI agent) works on this repo. **It is also
the content to paste into Claude Project Instructions** on claude.ai (so a chat
session outside the repo shares the same context).

---

## Identity & goal

- **Owner**: Thuan (devguy201-9). 5 years backend, HCMC. C# + Power Platform → transitioning to Go/Rust.
- **Project**: `checkout-saga` — distributed e-commerce checkout, 5 services, target: apply for mid-to-senior Go backend roles.

## Working agreement

**Chat language**: English, keeping technical terms in English (goroutine, saga,
idempotency, pessimistic lock, outbox, FOR UPDATE, gRPC, etc.). Do not translate them.
**All repo docs and code comments are written in English.**

**Style**:
- Sequential, step-by-step; one focused chunk at a time; a concrete, runnable verify checkpoint.
- Code comes with 1-2 lines explaining the trade-off / design reason.
- For decisions: "Chose X because ... instead of Y because ...".
- ASCII diagrams > prose when describing a flow.
- Don't over-explain. No emoji. No filler bullets.

## Engineering principles (NON-NEGOTIABLE)

1. **YAGNI**: build only features in the current phase. Don't build "just in case".
2. **KISS**: pick the simplest solution that can be defended.
3. **DRY**: extract to `pkg/` when 2+ services use the same pattern, not before.
4. **File size < 200 LOC**: split the file if it exceeds. Kebab-case filename describing its purpose.
5. **No new "enhanced" files**: edit existing files, don't create `file_v2.go`.
6. **Always compile-check after edits**: `go build ./...` must be clean before commit.
7. **Do not introduce concepts that cannot be defended deeply**:
   - DDD aggregate root: not yet mastered (lesson from the Vulcan interview) → avoid mentioning unless reviewed.
   - Every pattern used in code must have an ADR explaining the reason.

## Idiomatic Go (required)

- `context.Context` on EVERY DB call, HTTP handler, gRPC method.
- `pgx/v5` (NOT `database/sql + lib/pq`).
- `signal.NotifyContext` for graceful shutdown.
- Interfaces on the consumer side (Go idiom), not the producer side.
- Error wrap: `fmt.Errorf("scope: action: %w", err)`. Sentinel errors with `errors.Is/As`.
- `zap` for structured logging. Every log line must carry a `trace_id` field.
- Avoid GORM. SQL directly with `pgx` + `squirrel` builder.
- Avoid global state. DI via constructor.

## Test discipline

- Unit-test every usecase (mockable repo interface).
- Race-condition test for concurrency code (e.g. inventory FOR UPDATE).
- Integration tests with `testcontainers-go` (real PostgreSQL/Redis/Kafka) from the testing phase.
- Contract tests for gRPC proto once gRPC lands.
- Coverage target: usecase layer > 75%, repo layer > 50%.
- Do NOT fake data or skip tests to pass CI.

## Commit discipline

- Conventional Commits: `<type>(<scope>): <subject>` (e.g. `feat(order): add idempotency key`).
- Types: `feat | fix | refactor | test | docs | chore | perf | ci | build`.
- Body: WHAT + WHY, not WHO. Do not mention the AI assistant in the commit message.
- Small, frequent commits.

## Per-chunk output checklist

At the end of each chunk, the AI must deliver:

1. Real code (`.go` files in the repo, not inline paste).
2. Update the `Makefile` if there is a new target.
3. `docs/checkpoints/<topic>.md` — sequential verify commands.
4. A new ADR if there is a notable decision (`docs/adr/NNNN-*.md`).
5. A zip download.
6. A suggested commit message in Conventional Commits.
7. 2-3 "trade-offs chosen" bullets — in case an interviewer asks.

## Lessons from past interviews (REMEMBER)

- **In-memory KV transaction**: stack-based snapshot — BEGIN pushes, SET modifies, COMMIT pops-and-keeps, ROLLBACK pops-and-restores.
- **DDD aggregate**: not yet mastered → avoid bringing it up.
- **Argon2**: salt = per-user random, pepper = global secret.
- **Redis cache invalidation**: event-driven DEL on write, not scheduled sweeps.
- **Notification fan-out**: eager vs lazy — know when to use which.
- **Salary**: keep current compensation confidential; anchor 25-28M target.

## Honest interview framing

- "This is a personal project; I have not handled production incidents."
- "I have not worked with a senior Go team — that's why I'm applying at mid level."
- Don't oversell ("100k RPS production" — a reviewer knows it's personal).
- Don't bluff: a topic not in the project → "I haven't done it, only read the theory."

## Docs structure (always read before implementing)

```
docs/
├── project-overview-pdr.md   # Vision, scope, success criteria
├── system-architecture.md    # High-level, component interaction
├── code-standards.md         # Go style, patterns, conventions
├── codebase-summary.md       # Where things live (rolling update)
├── adr/                      # Architecture Decision Records
└── checkpoints/              # Per-topic verify steps
```

**Before implementing any feature**, read `./README.md` → the relevant `docs/*.md` → the related ADRs.

---

> Structural patterns adapted from go-clean-template, tailored for a single-owner monorepo of 5 services.
