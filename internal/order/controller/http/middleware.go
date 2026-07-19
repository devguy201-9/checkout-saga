package http

import (
	"crypto/rand"
	"encoding/hex"
	nethttp "net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/devguy201-9/checkout-saga/pkg/jwt"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// _traceHeader lets a caller supply its own id so one checkout can be followed
// across services; when absent we mint one.
const _traceHeader = "X-Request-ID"

// _authHeader / _bearerPrefix: the identity arrives as `Authorization: Bearer <token>`.
const (
	_authHeader   = "Authorization"
	_bearerPrefix = "Bearer "
)

// authMiddleware verifies the bearer token and puts the authenticated user id in
// the request context. It sits INSIDE withTraceID, so a rejected request is
// still traced. Failures return the same {error:{code,message}} envelope as the
// rest of the API — codes: missing_token, invalid_token.
func authMiddleware(jwtMgr *jwt.Manager, log logger.Logger) func(nethttp.Handler) nethttp.Handler {
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			header := r.Header.Get(_authHeader)
			if !strings.HasPrefix(header, _bearerPrefix) {
				writeError(w, log, nethttp.StatusUnauthorized,
					"missing_token", "authorization header with a bearer token is required")

				return
			}

			token := strings.TrimSpace(strings.TrimPrefix(header, _bearerPrefix))

			userID, err := jwtMgr.Parse(token)
			if err != nil {
				// The detail (why it failed) is intentionally not returned to the
				// caller — it only helps an attacker probe the token.
				writeError(w, log, nethttp.StatusUnauthorized,
					"invalid_token", "the provided token is not valid")

				return
			}

			ctx := WithUserID(r.Context(), userID)
			log.Debug(
				"request authenticated",
				zap.String("user_id", userID),
				zap.String("trace_id", logger.TraceIDFromContext(ctx)),
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// withTraceID puts a trace id in the request context, so every log line in a
// request scope can carry trace_id (docs/code-standards.md). It also echoes the
// id back, which makes a failing curl traceable in the logs.
//
// This is the seam where OpenTelemetry propagation slots in later.
func withTraceID(next nethttp.Handler) nethttp.Handler {
	return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		traceID := r.Header.Get(_traceHeader)
		if traceID == "" {
			traceID = newTraceID()
		}

		w.Header().Set(_traceHeader, traceID)
		next.ServeHTTP(w, r.WithContext(logger.WithTraceID(r.Context(), traceID)))
	})
}

// newTraceID returns 16 random bytes as hex. crypto/rand, not math/rand: ids
// leak into logs and headers, so they should not be predictable.
func newTraceID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// rand.Read on a healthy system does not fail; degrade rather than
		// refuse to serve the request.
		return "unknown"
	}

	return hex.EncodeToString(buf[:])
}
