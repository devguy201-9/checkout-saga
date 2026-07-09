# ADR-0003: Structured logging with zap, trace_id propagated via context

- **Status**: Accepted
- **Deciders**: Thuan
- **Tags**: `observability`, `logging`

## Context

We want a structured logger from the start, not later. Reasons:

- Senior-grade execution: observability is not bolted on.
- When debugging a saga flow across 5 services, **trace_id correlation** is essential.
- JSON format for prod (Cloud Logging parses it automatically), human-readable for dev.

Options:
1. `log/slog` (stdlib since Go 1.21)
2. `zap` (Uber)
3. `zerolog` (rs/zerolog)

## Decision

Use **`go.uber.org/zap`** behind a wrapper interface in `pkg/logger`. `trace_id`
is propagated via `context.Context`, and every log line in a request scope MUST
carry a `trace_id` field.

## Alternatives considered

### A. `log/slog` (stdlib)

- Pros: stdlib, no extra dependency, idiomatic for Go 1.21+.
- Cons: thinner ecosystem than zap (e.g. less mature OpenTelemetry bridge); ~30% slower in benchmarks.
- Why rejected: zap is more mature for production observability today, even though slog is the future direction.

### B. `zap` (chosen)

- Pros: industry standard for Go services, fast (allocation-free on the hot path), excellent OpenTelemetry integration, JSON + console encoders built in.
- Cons: one extra dependency; a more verbose API than slog.
- Why chosen: production maturity + performance + ecosystem.

### C. `zerolog`

- Pros: even faster than zap, fluent API.
- Cons: less common in Go monorepo conventions; the fluent API is harder to chain in error paths.
- Why rejected: marginal performance benefit; zap is the more conventional choice for interview discussion.

## Consequences

### Positive

- All services use the same `pkg/logger.Logger` interface — a consistent log format.
- `trace_id` is injected at the HTTP/gRPC middleware level and propagated to every downstream call.
- GCP Cloud Logging parses the JSON automatically → searchable by `trace_id`.

### Negative / Trade-offs

- Any function that passes ctx must extract trace_id when logging — boilerplate.
- The interface wrapper adds a small overhead versus using raw `*zap.Logger`.

### Mitigations

- A helper `logger.FromContext(ctx)` extracts trace_id and returns a scoped logger.
- The interface adds only one layer of indirection — negligible on the hot path.

## Design

```go
// pkg/logger/logger.go
type Logger interface {
    Debug(msg string, fields ...zap.Field)
    Info(msg string, fields ...zap.Field)
    Warn(msg string, fields ...zap.Field)
    Error(msg string, fields ...zap.Field)
    Fatal(msg string, fields ...zap.Field)
    With(fields ...zap.Field) Logger
    Sync() error
}

// Context propagation
func WithTraceID(ctx context.Context, traceID string) context.Context
func FromContext(ctx context.Context) Logger  // returns base logger + trace_id field
```

## Interview defense

> "I chose zap with context-aware trace_id propagation. Every log line in a request scope carries a trace_id field, and Cloud Logging parses JSON automatically → I can find one request across 5 services with a single query. I do not use slog because zap is more mature for OpenTelemetry integration right now — slog is the direction, but zap is still the default in the Go production stack."

## References

- [zap benchmarks](https://github.com/uber-go/zap#performance)
- [Cloud Logging structured logs](https://cloud.google.com/logging/docs/structured-logging)
- [OpenTelemetry Go logging](https://opentelemetry.io/docs/languages/go/)
