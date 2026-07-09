# Project Overview — Product Design Requirements (PDR)

## Vision

Build one production-grade distributed checkout system that **demonstrates the
ability to ship complex distributed systems** with senior-grade execution.
Target audience: hiring managers for mid-to-senior Go backend roles at product
companies in Vietnam.

## Out of scope

- Frontend UI (API only)
- Multi-tenant
- A real payment gateway (mock gateway)
- Internationalization
- A custom microservices framework (use off-the-shelf libraries)
- Kubernetes (Cloud Run is enough for this scope)

## Success criteria

The project is successful when:

1. **Functional**: the end-to-end checkout happy path + 6 failure scenarios all pass tests.
2. **Resilient**: killing any service mid-transaction → the system recovers, no inconsistent state.
3. **Performant**: p99 < 500ms at 100 RPS sustained (verified with k6).
4. **Observable**: trace one checkout request across 5 services in Jaeger; the checkout success-rate metric is visible in Grafana.
5. **Deployed**: live on GCP Cloud Run; push to main → auto-deploy in under 5 minutes.
6. **Documented**: a stranger can clone the repo → `docker-compose up` runs in 5 minutes. README is complete.
7. **Interview-ready**: 10+ concrete stories (race-condition discovery, cache stampede, saga recovery, ...) with real numbers.

## Core patterns demonstrated

| Pattern | Service | Reason |
|---|---|---|
| Saga orchestration | Saga Orchestrator | Distributed transaction across 4 services |
| Outbox pattern | Order, Saga | Atomicity between a business write and event publishing |
| Pessimistic lock (FOR UPDATE) | Inventory | Prevent oversell under high concurrency |
| Optimistic lock (version) | Order, Saga | Low-conflict updates |
| Idempotency key | Order, Payment | Prevent duplicate requests |
| Circuit breaker | Payment | Fail-fast when the gateway is slow |
| Exponential backoff + jitter | Payment | Retry transient errors safely |
| Singleflight | Order (product read) | Prevent cache stampede |
| Event-driven cache invalidation | Inventory | Stale cache when stock changes |
| FOR UPDATE SKIP LOCKED | Inventory cron, Outbox poller | Multi-worker safe claim |

## Stack constraints

- Go 1.22+
- PostgreSQL 16 (4 DBs, 1 instance for dev)
- Redis 7
- Kafka (Confluent Cloud free tier or local KRaft)
- gRPC between services, REST for external
- GCP Cloud Run (cloud), docker-compose (local)
- Prometheus + Grafana + OpenTelemetry + Jaeger

## Non-functional requirements

| NFR | Target | How verified |
|---|---|---|
| Latency p99 | < 500ms | k6 load test |
| Error rate | < 1% under load | k6 thresholds |
| Test coverage (usecase) | > 75% | `make cover-func` |
| Build time | < 60s for all 5 services | CI metric |
| Cold start | < 3s on Cloud Run | GCP metric |
| Lint clean | golangci-lint v2 strict, zero issues | CI gate |
| Vuln scan | govulncheck zero high/critical | CI gate |

## Scope (capability areas)

| Area | Deliverable |
|---|---|
| Foundation + Order Service | Docker stack + idempotency + JWT |
| Inventory + Pessimistic Lock | No-oversell guarantee |
| Payment + Resilience | Circuit breaker + retry + idempotency |
| Saga Orchestrator (core) | State machine + compensation + recovery |
| Outbox + Kafka | Event-driven + Notification consumer |
| Testing + Performance | Coverage > 75%, k6 baseline, pprof tuning |
| GCP Deploy + CI/CD | Live URL, auto-deploy on push |
| Observability | Prometheus + Jaeger + docs |

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Over-engineering (DDD/CQRS) that cannot be defended | An ADR for every decision; only patterns understood deeply |
| Saga complexity exceeds debuggability | Persist state in DB; the state-machine table keeps history |
| Scope creep | Strict timeline; weekly review skips non-essentials |
| GCP free tier exhausted | Monitor billing daily; pause Cloud Run when not demoing |
| An interview happens mid-build → CV not yet convincing | Update the CV regularly, link the work-in-progress GitHub |
