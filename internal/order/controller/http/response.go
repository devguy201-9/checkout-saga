package http

import (
	"encoding/json"
	"fmt"
	"io"
	nethttp "net/http"

	"go.uber.org/zap"

	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// _maxBodyBytes caps request bodies (1 MiB): an unbounded read is a trivial
// memory-exhaustion vector.
const _maxBodyBytes = 1 << 20

// envelope keeps one consistent response shape: { "data": ... } on success,
// { "error": { code, message } } on failure. Clients parse one structure.
type envelope struct {
	Data  any        `json:"data,omitempty"`
	Error *errorBody `json:"error,omitempty"`
}

// errorBody carries a stable machine-readable code plus a human message.
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w nethttp.ResponseWriter, log logger.Logger, status int, data any) {
	write(w, log, status, envelope{Data: data})
}

func writeError(w nethttp.ResponseWriter, log logger.Logger, status int, code, message string) {
	write(w, log, status, envelope{Error: &errorBody{Code: code, Message: message}})
}

func write(w nethttp.ResponseWriter, log logger.Logger, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	// Headers are already sent, so a write failure can only be logged.
	if err := json.NewEncoder(w).Encode(body); err != nil {
		log.Error("write response", zap.Error(err))
	}
}

// decodeJSON reads a bounded body and rejects unknown fields, so a typo like
// "quantiy" is a 400 instead of a silently-zero value.
func decodeJSON(r *nethttp.Request, dst any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, _maxBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}

	return nil
}
