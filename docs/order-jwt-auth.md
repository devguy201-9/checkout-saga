# Plan: JWT Authentication

Actionable spec for the auth chunk. Drive Claude Code with it (`/spec:create` on
this file, then validate/decompose/execute), then review every line. Follows
`docs/code-standards.md` and the layering in
`docs/adr/0002-clean-architecture-per-service.md`.

## Context

`POST /orders` currently reads `user_id` from the request body, so any client can
place an order on anyone's behalf. Identity must come from a verified token, not
the payload. This chunk adds JWT verification and moves `user_id` from the body
into the request context.

## Goal

A request without a valid `Authorization: Bearer <token>` is rejected `401`;
a valid token puts `user_id` in the context; `POST /orders` no longer accepts
`user_id` in the body and uses the authenticated identity. Verifiable: the four
token cases (valid / expired / bad-signature / missing) return the expected codes,
and a body carrying `user_id` is ignored.

## Out of scope

- Refresh tokens, token revocation / blacklist, and a real user store.
- Password auth / registration (the `/login` endpoint is a mock issuer for demo).
- Role/permission authorization (authn only, not authz).
- Rate limiting.

## New dependency

`github.com/golang-jwt/jwt/v5` — the standard, maintained JWT library. One
dependency, justified in ADR-0006. Run `go get github.com/golang-jwt/jwt/v5`.

## Tasks

- [x] `pkg/config`: add an `Auth` block — `JWTSecret string`
      (`env:"JWT_SECRET,required" validate:"required,min=32"`) and
      `JWTExpiry time.Duration` (`env:"JWT_TOKEN_EXPIRY" envDefault:"24h"`).
      Both env vars already exist in `.env.example`.
- [x] `internal/order/app/config.go`: embed `config.Auth` in the order `Config`.
- [x] `pkg/jwt`: a small `Manager` (constructed from secret + expiry, no globals):
      - `Generate(userID string) (string, error)` — HS256, sets `sub`, `iat`,
        `exp`; `RegisteredClaims`.
      - `Parse(token string) (userID string, err error)` — verifies signature +
        expiry, enforces the signing method is HMAC (reject `alg: none` and any
        non-HMAC alg — the classic JWT bypass), returns a sentinel
        `ErrInvalidToken` for all failures so callers don't branch on library
        internals.
- [x] `internal/order/controller/http/middleware.go`: add `authMiddleware(jwtMgr, log)`:
      - read `Authorization`, require the `Bearer ` prefix,
      - `Parse` the token; on any error → `401` via the `{error}` envelope,
      - put `user_id` into the context (`logger.WithTraceID` already shows the
        context pattern; add a `userIDKey`), and add `user_id` as a log field for
        the request scope.
- [x] `internal/order/controller/http/context.go` (new small file): unexported
      `userIDKey` + `WithUserID` / `UserIDFromContext` helpers.
- [x] `router.go`: apply `authMiddleware` to `POST /orders` and `GET /orders/{id}`,
      but NOT to `POST /login` and NOT to `/health` (mount those before/around
      auth). Keep `withTraceID` outermost so even rejected requests are traced.
- [x] `order-handler.go` (`create`): stop reading `req.UserID`; take the id from
      `UserIDFromContext(ctx)`.
- [x] `dto.go`: remove `UserID` from `createOrderRequest` (and its
      `validate:"required,uuid"`); `toEntityItems` is unchanged.
- [x] `login-handler.go` (new): `POST /login` accepts `{"user_id":"<uuid>"}`,
      validates the uuid, returns `{"data":{"token":"..."}}`. Clearly a DEV MOCK
      issuer (no password) — comment says so.
- [x] `app.go`: build the `jwt.Manager` from config and pass it into `NewRouter`.
- [x] Tests:
      - `pkg/jwt`: generate→parse round trip; expired token (construct with a past
        expiry or a 0 expiry manager); tampered token; wrong secret; `alg:none`
        rejected.
      - `controller/http`: middleware table test (valid / missing header /
        malformed / expired / bad signature) using a real `jwt.Manager` and an
        `httptest` recorder; assert 401 code + envelope on failures, and that a
        valid token reaches a stub handler with the right `user_id`.
- [x] `docs/adr/0006-jwt-stateless-auth.md`.
- [x] `docs/codebase-summary.md`: `pkg/jwt` present; note auth middleware.
- [x] `docs/checkpoints/order-jwt-auth.md`: sequential verify (below).

## Design constraints (do not deviate without an ADR)

- HS256 (symmetric) is fine for a single issuer+verifier. Note in the ADR that
  RS256 (asymmetric) is what you'd use when multiple services verify tokens they
  didn't issue — relevant once the saga/other services need to check identity.
- Enforce the signing method inside `Parse` (`token.Method.(*jwt.SigningMethodHMAC)`);
  a parser that trusts the token's `alg` header is the well-known forgery hole.
- Secret comes from env, validated `min=32`; never hardcoded. `config.Load`
  already fails fast when a `required` env var is missing.
- The middleware returns the same `{error:{code,message}}` envelope as the rest of
  the API; codes: `missing_token`, `invalid_token`.

## Verify checkpoint

```bash
go mod tidy
make lint && make test            # pkg/jwt + middleware unit tests, no DB

make up && make migrate-up && make run-order

USER_ID=11111111-1111-1111-1111-111111111111
PRODUCT_ID=22222222-2222-2222-2222-222222222222

# 1) login (mock) -> token
TOKEN=$(curl -s -XPOST localhost:8081/login -H 'Content-Type: application/json' \
  -d "{\"user_id\":\"$USER_ID\"}" | sed -E 's/.*"token":"([^"]+)".*/\1/')

# 2) no token -> 401 missing_token
curl -i -XPOST localhost:8081/orders -H 'Idempotency-Key: k1' \
  -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":2,\"price\":\"10.50\"}]}"

# 3) valid token -> 201, user_id comes from the token (body has none)
curl -i -XPOST localhost:8081/orders -H "Authorization: Bearer $TOKEN" \
  -H 'Idempotency-Key: k1' -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":2,\"price\":\"10.50\"}]}"

# 4) tampered token -> 401 invalid_token
curl -i -XPOST localhost:8081/orders -H "Authorization: Bearer ${TOKEN}x" \
  -H 'Idempotency-Key: k2' -H 'Content-Type: application/json' \
  -d "{\"items\":[{\"product_id\":\"$PRODUCT_ID\",\"quantity\":1,\"price\":\"5.00\"}]}"

# 5) response order's user_id must equal $USER_ID
```

Expect: (2) 401 `missing_token`, (3) 201 with `data.user_id == $USER_ID`,
(4) 401 `invalid_token`. Confirm the created order's `user_id` is the token's
subject, not anything from the body.

## Risks / unknowns

- Expiry testing: allow the `Manager` to be constructed with a tiny/zero expiry in
  tests so an "expired" token is deterministic (no sleeping).
- Middleware ordering: `withTraceID` outermost, then `authMiddleware`, so a 401 is
  still traced. Mount `/login` and `/health` outside auth.
- Clock skew: `jwt/v5` has a small leeway option; default is fine for local.

## Notes during execution

- `Generate(userID, now)` takes `now` as a parameter (read at the composition edge
  in `login-handler.go`), keeping `pkg/jwt` clock-free and testable without
  `time.Sleep`. `Manager` accepts a negative/zero expiry so a deterministically
  expired token can be minted in tests.
- Defense-in-depth on the signing method: `keyFunc` type-asserts
  `*jwt.SigningMethodHMAC` (rejects before the signature is checked) AND
  `ParseWithClaims(..., WithValidMethods([]string{HS256}))` as a second layer.
- Above-spec hardening: `Parse` also rejects a validly-signed token whose `sub`
  is empty, so a blank identity can't slip through.
- Body-spoof closed at two layers: `user_id` removed from `createOrderRequest`,
  and `decodeJSON` uses `DisallowUnknownFields`, so a stray body `user_id` is a
  `400 invalid_body`, never silently used.
- No `/health` route exists yet, so only `/login` is mounted public (YAGNI — did
  not invent a health endpoint the task list didn't call for).
- Lint gotcha (golangci-lint v2 strict): tests need `t.Parallel()` (paralleltest)
  and `httptest.NewRequestWithContext` (noctx).
- Tampered-token test flake fixed: flipping the JWT's LAST base64 char sometimes
  only flips signature padding bits (the final char carries 4 meaningful bits of a
  256-bit HMAC). Mutate the FIRST char of the signature segment instead —
  deterministic. Stable over 20 race runs.

## Reflection

- **Done**: all tasks. `go build ./...`, `make lint` (0 issues), `make test`
  (race) green. `pkg/jwt` coverage 83.3%. One new dependency only
  (`golang-jwt/jwt/v5 v5.3.1`). Independent review: APPROVE 9.5/10, 0
  critical/high.
- **Next** (out of scope here, tracked in ADR-0006 + checkpoint): refresh tokens
  + revocation (needs a store), role/permission authz, RS256 migration once other
  services verify order-issued tokens, testcontainers integration tests over the
  authed endpoints.
