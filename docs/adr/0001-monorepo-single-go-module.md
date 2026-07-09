# ADR-0001: Monorepo with a single Go module

- **Status**: Accepted
- **Deciders**: Thuan
- **Tags**: `architecture`, `repo-structure`

## Context

5 services (order, inventory, payment, saga, notification) will share some code
(logger, postgres helper, errors, observability). We need to decide the structure:

1. **Multi-repo**: one repo per service
2. **Monorepo + multi-module** (Go workspaces): 5 separate `go.mod` files
3. **Monorepo + single module**: one `go.mod` at the root

Constraints: single-owner project, deploy to GCP Cloud Run, share `pkg/`.

## Decision

A **single-module monorepo** with 5 entry points in `cmd/<service>/main.go`.

## Alternatives considered

### A. Multi-repo (one repo per service)

- Pros: each service has its own version, CI, and release cycle.
- Cons: 5 repos is overhead for a single-owner project. Sharing code requires publishing a package or using submodules (painful). A reviewer browsing 5 repos gets a fragmented impression.
- Why rejected: the overhead is not justified at this scope.

### B. Monorepo + Go workspaces (multi-module)

- Pros: each service can be tagged independently; closer to a "real microservices" feel.
- Cons: more complexity (5 `go.mod` + `go.work`), dependency-drift risk, a more complex CI matrix, and harder tag/release coordination. Most "Go workspaces" examples online still share a single root module anyway.
- Why rejected: poor complexity-to-value ratio for this demo. A Cloud Run build from the root path does not require a module per service.

### C. Single-module monorepo (chosen)

- Pros: one `go.mod`, easy refactoring across services, a single `make lint`/`make test`, and a single multi-target Dockerfile via `--build-arg SERVICE=...`. A reviewer sees one repo with a clear structure.
- Cons: cannot release services independently with different versions. An internal API change breaks compilation across services (also a pro — breakage is caught early).
- Why chosen: the simplest pattern that serves the goal (demonstrate a distributed system); there is no benefit from independent versioning in a single-owner project.

## Consequences

### Positive

- Single `go.mod`; `go test ./...` works across all services.
- `pkg/` is shared trivially — no replace directives.
- The Dockerfile uses `--build-arg SERVICE=order` to pick the entry point.
- CI iterates over `cmd/<service>` for parallel builds.

### Negative / Trade-offs

- Cannot tag `order-service v1.2.3` while `payment-service v0.5.0`. All services share one git tag.
- A single `go.sum` — a vulnerability in one service's transitive dependency affects all.

### Mitigations

- For independent versioning (if ever needed): split a service's `cmd/` into its own repo at that point. YAGNI for now.
- For supply-chain safety: `govulncheck ./...` in CI applies to all services uniformly.

## Interview defense

> "I chose a single-module monorepo because the scope is one owner and 5 tightly related services (same deploy cycle, same team-of-one). Workspaces would add unnecessary overhead. The trade-off is that I cannot tag each service independently, but this project does not need independent releases. If a service later needs to evolve independently (e.g. open-sourcing one), I would split the repo at that point."

## References

- [Go workspaces design](https://go.dev/blog/get-familiar-with-workspaces)
- [Monorepo trade-offs (Google blog)](https://research.google/pubs/why-google-stores-billions-of-lines-of-code-in-a-single-repository/)
