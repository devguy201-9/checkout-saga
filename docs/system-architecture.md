# System Architecture

## High-level view

```
                       ┌─────────────────────┐
                       │   Client (mock)     │
                       │   curl / k6         │
                       └──────────┬──────────┘
                                  │ HTTPS + JWT
                                  ▼
                       ┌─────────────────────┐
                       │  Order Service      │
                       │  (REST entrypoint)  │
                       │  - Idempotency      │
                       │  - JWT auth         │
                       └──────────┬──────────┘
                                  │ gRPC
                                  ▼
                       ┌─────────────────────┐
                       │  Saga Orchestrator  │
                       │  (state machine)    │
                       │  - Compensation     │
                       │  - Recovery worker  │
                       └─┬────────┬────────┬─┘
                         │ gRPC   │ gRPC   │ gRPC
                ┌────────┘   ┌────┘    ┌───┘
                ▼            ▼         ▼
         ┌──────────┐  ┌─────────┐  ┌─────────────┐
         │Inventory │  │Payment  │  │  (future)   │
         │Service   │  │Service  │  │             │
         │          │  │+ CircB. │  │             │
         └────┬─────┘  └────┬────┘  └─────────────┘
              │             │
              │             │ events (outbox → Kafka)
              ▼             ▼
         ┌────────────────────────┐
         │   Kafka (Confluent)    │
         │  order.created         │
         │  order.completed       │
         │  order.cancelled       │
         │  payment.processed     │
         │  payment.refunded      │
         │  inventory.reserved    │
         │  inventory.released    │
         └───────────┬────────────┘
                     │ consume
                     ▼
            ┌─────────────────────┐
            │ Notification Service│
            │ (mock email/FCM)    │
            └─────────────────────┘

Storage (per service):
  ┌─────────────────────────────────────────────┐
  │ PostgreSQL 16 (1 instance, 4 DBs)           │
  │   order_db, inventory_db, payment_db,       │
  │   saga_db                                   │
  ├─────────────────────────────────────────────┤
  │ Redis 7 (shared cache, singleflight)        │
  └─────────────────────────────────────────────┘

Observability:
  Prometheus + Grafana | OpenTelemetry + Jaeger | structured logs
```

## Communication patterns

| From → To | Protocol | Reason |
|---|---|---|
| Client → Order | REST/JSON | Public-facing, browser-friendly |
| Order → Saga | gRPC | Internal, typed contracts, performance |
| Saga → Inventory/Payment | gRPC | Same as above |
| Services → Kafka | Outbox poller → Kafka producer | At-least-once delivery, atomic with DB |
| Notification → Kafka | Consumer | Async, decoupled |

## Why this topology

- **Saga orchestration** (not choreography): a single state machine is easier to debug and has clear retry semantics. Trade-off: tighter coupling — the orchestrator knows about downstream services.
- **One synchronous path** (Client → Order → Saga → downstream): the user sees success/fail immediately after checkout. Compensation runs asynchronously on failure.
- **Notification is async via Kafka**: email/push does not block checkout latency.
- **Per-service database**: data isolation, each service owns its data. Trade-off: no cross-service JOINs (the right microservices spirit).

## Failure modes & responses

| Failure | Response |
|---|---|
| Inventory insufficient | Reject immediately, no saga compensation needed |
| Payment fail | Saga compensates: release the inventory reservation |
| Payment success, inventory commit fail | Saga compensates: refund payment + release |
| Kafka down (publish fail) | Outbox keeps the event, poller retries; checkout still succeeds |
| Saga orchestrator crash mid-flow | Recovery worker scans `saga_instances` in an intermediate state > 5 min and resumes |
| Inventory reserve TTL expired (user did not pay) | Cron job releases the reservation after 10 min |
| Payment gateway slow (>30s) | Circuit breaker opens after a 60% failure rate, fails fast |
| Cache miss spike | Singleflight: one DB query for N concurrent requests |

## State machines

### Order status

```
PENDING ──► INVENTORY_RESERVED ──► PAYMENT_PROCESSING ──► COMPLETED
   │                                       │
   │                                       └──► PAYMENT_FAILED ──► CANCELLED
   │
   └──► INVENTORY_INSUFFICIENT ──► CANCELLED
```

### Saga state

```
STARTED
  ↓
INVENTORY_RESERVING ─► INVENTORY_RESERVED
  ↓
PAYMENT_PROCESSING ─► PAYMENT_COMPLETED
  ↓
INVENTORY_COMMITTING ─► INVENTORY_COMMITTED
  ↓
COMPLETED

Any step fails → COMPENSATING → COMPENSATED (or FAILED if compensation also fails)
```

## Deployment topology

```
GCP Project: checkout-saga-prod
├── Cloud Run (5 services, each its own URL)
│   ├── order-service       (public, allow-unauthenticated)
│   ├── saga-orchestrator   (internal, ingress=internal)
│   ├── inventory-service   (internal)
│   ├── payment-service     (internal)
│   └── notification-service (internal)
├── Cloud SQL (PostgreSQL 16, db-f1-micro)
│   └── 4 databases as in dev
├── Memorystore (Redis, basic tier, 1GB)
├── Confluent Cloud (Kafka, free tier cluster)
├── Secret Manager (DB password, JWT secret, Kafka creds)
├── Artifact Registry (Docker images)
└── Cloud Logging + Cloud Monitoring
```

## Non-functional details

- Connection pool: pgx `MaxConns=25` per service (configurable per env).
- Idempotency key TTL: 24h (after expiry, key reuse is allowed).
- Inventory reservation TTL: 10 min (auto-release via cron).
- Saga step timeout: 30s (then compensate).
- Circuit breaker: trip at ≥10 requests, ≥60% failure ratio, open for 30s.
- Outbox poller interval: 1s, batch size 100, `FOR UPDATE SKIP LOCKED`.

## Open questions

- gRPC mesh or direct calls? → direct calls (KISS, this scope).
- Synchronous replication for saga state? → No, a single primary is enough.
- Distributed-tracing sampler rate? → 10% prod, 100% dev.
