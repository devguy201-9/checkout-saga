# Checkout Saga

> Production-grade distributed e-commerce checkout system with Saga orchestration. Built to demonstrate senior backend engineering — Go, PostgreSQL, Kafka, gRPC, observability, and GCP Cloud Run deployment.

**Stack**: Go 1.22 · PostgreSQL 16 · Redis 7 · Kafka · gRPC · GCP Cloud Run

## Why this project

Distributed transactions are hard. This project demonstrates:

- **Saga orchestration** across 5 microservices with explicit compensating transactions
- **Outbox pattern** for atomicity between business writes and event publishing
- **Pessimistic locking** (`FOR UPDATE`) to prevent inventory oversell
- **Circuit breaker** and exponential backoff for payment gateway resilience
- **Singleflight** to prevent cache stampede
- **Idempotency keys** to prevent duplicate orders and double-charges
- **Observability** from the start: structured logs with `trace_id`, plus Prometheus metrics and OpenTelemetry tracing (planned)

Architecture detail in [`docs/system-architecture.md`](docs/system-architecture.md). Design rationale lives in [`docs/adr/`](docs/adr/) (Architecture Decision Records).

## Quick start

```bash
# 1. Setup
cp .env.example .env
go mod tidy

# 2. Infrastructure (PostgreSQL + Redis + Adminer)
make up
make verify

# 3. Build services
make build

# 4. Run a service locally
make run-order

# 5. Cleanup
make down
```

Adminer: <http://localhost:8090> · server: `postgres` · user/pwd from `.env`.

## Services

| Service | HTTP | gRPC | DB | Responsibility |
|---|---|---|---|---|
| order | 8081 | 9081 | order_db | Idempotent order creation, status lookup |
| inventory | 8082 | 9082 | inventory_db | Stock reservation with pessimistic lock |
| payment | 8083 | 9083 | payment_db | Mock gateway with circuit breaker |
| saga | 8084 | 9084 | saga_db | State machine orchestrating checkout |
| notification | 8085 | — | — | Kafka consumer, mock email/FCM |

## Project structure

```
checkout-saga/
├── CLAUDE.md                # Working agreement for AI agents
├── README.md                # ← you are here
├── cmd/<service>/main.go    # Service entry points (5 services)
├── internal/<service>/      # Service-specific code (Clean Architecture)
│   ├── app/                 # Composition root, DI, lifecycle
│   ├── entity/              # Domain types + invariants
│   ├── usecase/             # Business logic + repo interfaces
│   ├── repo/                # Data access implementations
│   └── controller/{http,grpc}/  # I/O adapters
├── pkg/                     # Cross-service shared (no business logic)
│   ├── logger/              # zap + trace_id propagation
│   ├── postgres/            # pgx/v5 pool, retry, health
│   ├── httpserver/          # (planned)
│   └── errors/              # (planned)
├── proto/v1/                # gRPC contracts (planned)
├── migrations/<service>/    # SQL migrations per DB
├── deployments/             # Docker/GCP configs
├── docs/                    # PDR + architecture + ADRs + checkpoints
├── tests/                   # Load (k6) + chaos (planned)
├── integration-test/        # testcontainers-based (planned)
├── .github/workflows/       # CI: lint + race + vuln + build matrix
├── .golangci.yml            # Strict linter config
├── Dockerfile               # Multi-target distroless build
├── docker-compose.yml       # Local dev infra
└── Makefile                 # Single entry point for tasks
```

## Documentation

Every technical decision is documented. Read in order:

1. [Project overview & PDR](docs/project-overview-pdr.md) — vision, scope, success criteria
2. [System architecture](docs/system-architecture.md) — high-level diagrams, communication patterns
3. [Code standards](docs/code-standards.md) — Go style, conventions, naming
4. [Codebase summary](docs/codebase-summary.md) — where things live (rolling update)
5. [ADRs](docs/adr/) — significant decisions with alternatives + trade-offs
6. [Checkpoints](docs/checkpoints/) — per-topic verify steps

## Quality gates (CI)

Every push to `main` runs:

- `golangci-lint` v2 with strict config (`cyclop`, `gocognit`, `gosec`, `errorlint`, ...)
- `go test -race -covermode=atomic` across all packages
- `govulncheck` for known vulnerabilities
- Build matrix across 5 services

Local equivalent: `make pre-commit`.

## Scope

Capability areas the project builds out:

- Foundation + Order Service (JWT, idempotency)
- Inventory with pessimistic locking (no oversell)
- Payment with circuit breaker + retry
- Saga Orchestrator (state machine + compensation + recovery)
- Outbox + Kafka + Notification consumer
- Testing + performance tuning
- GCP Cloud Run deploy + CI/CD
- Observability (Prometheus + Jaeger)

## License

MIT — see [LICENSE](LICENSE).

## Acknowledgements

- Structural patterns inspired by [evrone/go-clean-template](https://github.com/evrone/go-clean-template).
