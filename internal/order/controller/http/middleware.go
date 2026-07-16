package http

import (
	"crypto/rand"
	"encoding/hex"
	nethttp "net/http"

	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// _traceHeader lets a caller supply its own id so one checkout can be followed
// across services; when absent we mint one.
const _traceHeader = "X-Request-ID"

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
