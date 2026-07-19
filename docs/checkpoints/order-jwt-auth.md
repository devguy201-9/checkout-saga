# Order JWT Authentication Checkpoint

**Theme**: `pkg/jwt` + auth middleware. `POST /orders` and `GET /orders/{id}`
require a verified `Authorization: Bearer <token>`; identity comes from the token,
not the body. `POST /login` is a dev mock token issuer.

## What this covers

**Shared package**:
- `pkg/jwt` — `Manager` (HS256, DI, no globals). `Generate(userID, now)` and
  `Parse(token) -> userID`. `Parse` enforces the HMAC signing method (rejects
  `alg:none` and any non-HMAC alg) and returns the sentinel `ErrInvalidToken` for
  every failure.

**Config**:
- `config.Auth` — `JWT_SECRET` (`required`, `min=32`), `JWT_TOKEN_EXPIRY`
  (default `24h`). Embedded in the order service `Config`. `config.Load` fails
  fast when the secret is missing or too short.

**Order service (controller/http)**:
- `context.go` — `WithUserID` / `UserIDFromContext` (unexported struct key).
- `middleware.go` — `authMiddleware(jwtMgr, log)`: Bearer parse → `Parse` →
  inject `user_id` → `next`. Failures return the `{error:{code,message}}`
  envelope (`missing_token`, `invalid_token`). Sits inside `withTraceID`, so a
  401 is still traced.
- `router.go` — auth applied per protected route; `/login` stays public.
- `order-handler.go` — `create` takes the id from `UserIDFromContext(ctx)`.
- `dto.go` — `createOrderRequest` no longer has `user_id`.
- `login-handler.go` — `POST /login` (dev mock) mints a token for a uuid.
- `app.go` — builds `jwt.Manager` from config, injects into `NewRouter`.

**Docs**:
- `docs/adr/0006-jwt-stateless-auth.md`
- `docs/codebase-summary.md` — statuses updated.

## Endpoints

| Method | Path | Auth | Success | Notes |
|---|---|---|---|---|
| POST | `/login` | public | 200 `{data:{token}}` | dev mock issuer, no password |
| POST | `/orders` | bearer | 201 / 200 replay | `user_id` from token, `Idempotency-Key` header |
| GET | `/orders/{id}` | bearer | 200 | 404 unknown, 400 non-uuid |

## Verify checkpoint — sequential

Run in order; each step must pass before the next.

### 1. Build + unit tests (no DB needed)

```bash
go mod tidy
make lint            # golangci-lint v2 strict — must be 0 issues
make test            # pkg/jwt round-trip/expired/tampered/wrong-secret/alg:none
                     # + controller/http middleware table test
```

Expect: `pkg/jwt` and `internal/order/controller/http` PASS with race on.

### 2. Infra + schema + run

```bash
make up && make migrate-up && make run-order
```

Expect the `order service ready` line on `:8081`.

### 3. Login (mock) → token

In a second terminal:

```bash
USER_ID=11111111-1111-1111-1111-111111111111
PRODUCT_ID=22222222-2222-2222-2222-222222222222

TOKEN=$(curl -s -XPOST localhost:8081/login -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\"}" | sed -E 's/.*"token":"([^"]+)".*/\1/')
echo "$TOKEN"
```

Expect a non-empty three-segment token (`header.payload.signature`).

### 4. No token → 401 missing_token

```bash
curl -i -XPOST localhost:8081/orders -H 'Idempotency-Key: k1' \
  -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":2,\"price\":\"10.50\"}]}"
```

Expect `401` with `error.code == "missing_token"`.

### 5. [IMPORTANT] Valid token → 201, identity from the token

The body carries NO `user_id`.

```bash
curl -i -XPOST localhost:8081/orders -H "Authorization: Bearer $TOKEN" \
  -H 'Idempotency-Key: k1' -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":2,\"price\":\"10.50\"}]}"
```

Expect `201`, and `data.user_id == $USER_ID` — the token's subject, proving the
identity is not taken from the payload.

### 6. Tampered token → 401 invalid_token

```bash
curl -i -XPOST localhost:8081/orders -H "Authorization: Bearer ${TOKEN}x" \
  -H 'Idempotency-Key: k2' -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"price\":\"5.00\"}]}"
```

Expect `401` with `error.code == "invalid_token"`.

### 7. [IMPORTANT] A body `user_id` is ignored

`DisallowUnknownFields` makes a stray `user_id` a hard error rather than a silent
override — confirm the body cannot inject identity:

```bash
curl -i -XPOST localhost:8081/orders -H "Authorization: Bearer $TOKEN" \
  -H 'Idempotency-Key: k3' -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"99999999-9999-9999-9999-999999999999\",\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"price\":\"5.00\"}]}"
```

Expect `400 invalid_body` (unknown field). The order can never be attributed to
the body's `user_id`.

## Common pitfalls

- **`JWT_SECRET` too short**: the service fails to start — `config.Load` enforces
  `min=32`. This is intended; lengthen the secret in `.env`.
- **`401 invalid_token` on a fresh token**: the issuer and verifier must share the
  same `JWT_SECRET`. A restart with a different secret invalidates old tokens.
- **`401 missing_token` with a token present**: the header must be exactly
  `Authorization: Bearer <token>` (the `Bearer ` prefix is required).
- **`/login` returns a token without a password**: by design — it is a dev mock
  issuer. Do not ship it as-is.

## Trade-offs chosen

1. **HS256 (symmetric) not RS256** — one issuer that is also the only verifier, so
   a shared secret is enough. RS256 lands when other services verify these tokens
   (ADR-0006).
2. **Signing method enforced inside `Parse`** — trusting the token's own `alg`
   header is the `alg:none` / alg-confusion forgery; the type assertion to
   `*SigningMethodHMAC` closes it, `WithValidMethods` backs it up.
3. **Single `ErrInvalidToken` sentinel** — the HTTP layer branches on one domain
   error, never on `golang-jwt` internals; the failure reason is logged, not
   returned (it only helps an attacker probe).
4. **`user_id` removed from the DTO, not just ignored** — the wire contract makes
   spoofing structurally impossible rather than relying on a handler to skip it.
5. **`/login` as a dev mock** — issuing is decoupled from verifying so the auth
   flow is demoable now; the real issuer (password/OAuth + user store) is a later
   chunk.

## Next

- Refresh tokens + revocation (needs a store) and role/permission authz.
- RS256 migration once the saga / other services verify order-issued tokens.
- Integration tests (testcontainers) exercising the authed endpoints end-to-end.
