// Package http contains the REST controllers of the order service. It maps
// HTTP to usecase calls and back — no business logic lives here.
//
// The stdlib is imported as nethttp because this package is itself named http.
package http

import (
	nethttp "net/http"

	"github.com/devguy201-9/checkout-saga/internal/order/usecase"
	"github.com/devguy201-9/checkout-saga/pkg/jwt"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// NewRouter wires the order endpoints and the request-scoped middleware.
//
// Routing uses the stdlib ServeMux with Go 1.22 method+path patterns
// ("POST /orders", "GET /orders/{id}"). That covers what this service needs, so
// no third-party router is pulled in (KISS + no dependency to defend).
//
// Auth is applied PER protected route rather than to the whole mux, so the
// public /login (the dev token issuer) stays open. withTraceID wraps the whole
// mux and therefore stays outermost — even a 401 from auth is traced.
func NewRouter(orderUseCase *usecase.OrderUseCase, jwtMgr *jwt.Manager, log logger.Logger) nethttp.Handler {
	orderHandler := newOrderHandler(orderUseCase, log)
	loginHandler := newLoginHandler(jwtMgr, log)
	auth := authMiddleware(jwtMgr, log)

	mux := nethttp.NewServeMux()

	// Public: mints a demo token, so it must be reachable without one.
	mux.HandleFunc("POST /login", loginHandler.issue)

	// Protected: identity must come from a verified token, not the body.
	mux.Handle("POST /orders", auth(nethttp.HandlerFunc(orderHandler.create)))
	mux.Handle("GET /orders/{id}", auth(nethttp.HandlerFunc(orderHandler.getByID)))

	return withTraceID(mux)
}
