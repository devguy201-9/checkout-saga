# ADR-0006: Stateless JWT authentication (HS256)

- **Status**: Accepted
- **Deciders**: Thuan
- **Tags**: `security`, `auth`, `dependency`

## Context

`POST /orders` read `user_id` from the request body, so any client could place an
order on anyone's behalf. Identity must come from a verified credential, not the
payload. The order service is a single issuer and verifier of its own tokens, and
there is no session store. We need authentication (authn) only — authorization,
refresh tokens, and a real user store are out of scope for this chunk.

## Decision

Use **stateless JWTs signed with HS256** (symmetric HMAC-SHA256). A small
`pkg/jwt.Manager` — constructed from a secret + expiry, injected via DI, no
globals — issues and verifies tokens. `Parse` **enforces the signing method is
HMAC** and collapses every failure to a single sentinel `ErrInvalidToken`. Auth
middleware puts the token subject in the request context; the handler reads it
from there. One new dependency: `github.com/golang-jwt/jwt/v5`.

## Alternatives considered

### A. Server-side sessions (opaque token + store)

- Pros: instant revocation; no token forgery surface; small tokens.
- Cons: every request hits the session store (Redis/DB) — stateful, another
  round trip, another thing to run. Overkill for a demo with no logout story.
- Why rejected: statelessness is the point here; revocation is explicitly out of
  scope, so the session store buys nothing yet.

### B. RS256 (asymmetric) instead of HS256

- Pros: verifiers hold only the public key; the private signing key never leaves
  the issuer. This is the right choice once *other* services verify tokens the
  order service issued (the saga will).
- Cons: key-pair management and distribution overhead for a single issuer that is
  also the only verifier — no benefit today.
- Why rejected for now: YAGNI. HS256 is correct for one issuer+verifier. When a
  second service needs to verify identity, revisit with a superseding ADR and
  migrate to RS256 (only `pkg/jwt` and key distribution change).

### C. Hand-rolled token / HMAC scheme

- Pros: zero dependencies.
- Cons: reinventing a security primitive — expiry parsing, constant-time compare,
  alg handling — is exactly where footguns live.
- Why rejected: `golang-jwt/jwt/v5` is the standard, maintained library; one
  well-understood dependency beats bespoke crypto plumbing.

## Consequences

### Positive

- No session store: verification is a signature check, fully stateless.
- The `alg` confusion / `alg:none` forgery is closed — `Parse` rejects any
  non-HMAC method before trusting the token, and `WithValidMethods` constrains the
  parser as a second layer.
- Identity can never be spoofed via the body: the DTO no longer has `user_id`, and
  `DisallowUnknownFields` turns a stray `user_id` into a 400.
- `pkg/jwt` is clock-free (`now` is passed in), so expiry is unit-testable without
  `time.Sleep`.

### Negative / Trade-offs

- No revocation: a leaked token is valid until it expires. Acceptable given the
  scope; mitigated by a short expiry (`JWT_TOKEN_EXPIRY`, default 24h).
- Secret sharing: HS256's secret is both sign and verify key, so every verifier
  must hold it. This is the reason to move to RS256 once multiple services verify.
- The `/login` endpoint is a **dev mock issuer** (no password) — clearly marked in
  code and out of scope to replace here.

### Mitigations

- Secret from env, validated `min=32` at config load (`config.Auth`); the service
  fails fast if it is missing or too short — never hardcoded.
- Short default expiry limits the blast radius of a leaked token until revocation
  or refresh is introduced.

## Interview defense

> "I used stateless HS256 JWTs because the order service issues and verifies its
> own tokens — no session store, verification is just a signature check. The
> trade-off is no revocation and a shared secret, which I accept because revocation
> is out of scope and there's a single verifier. The moment another service has to
> verify these tokens I'd move to RS256 so only the public key travels. The
> security-critical detail is that `Parse` enforces the HMAC signing method — a
> parser that trusts the token's own `alg` header is the classic `alg:none` /
> alg-confusion forgery."

## References

- [golang-jwt/jwt/v5](https://github.com/golang-jwt/jwt)
- [Critical vulnerabilities in JSON Web Token libraries (alg confusion)](https://auth0.com/blog/critical-vulnerabilities-in-json-web-token-libraries/)
- ADR-0002 (layering: middleware in `controller/http`, config in `app`), ADR-0003
  (trace_id on every request-scoped log line).
