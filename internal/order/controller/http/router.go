// Package http contains the REST controllers of the order service. It maps
// HTTP to usecase calls and back — no business logic lives here.
//
// The stdlib is imported as nethttp because this package is itself named http.
package http

import (
	nethttp "net/http"

	"github.com/devguy201-9/checkout-saga/internal/order/usecase"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// NewRouter wires the order endpoints and the request-scoped middleware.
//
// Routing uses the stdlib ServeMux with Go 1.22 method+path patterns
// ("POST /orders", "GET /orders/{id}"). That covers what this service needs, so
// no third-party router is pulled in (KISS + no dependency to defend).
func NewRouter(orderUseCase *usecase.OrderUseCase, log logger.Logger) nethttp.Handler {
	handler := newOrderHandler(orderUseCase, log)

	mux := nethttp.NewServeMux()
	mux.HandleFunc("POST /orders", handler.create)
	mux.HandleFunc("GET /orders/{id}", handler.getByID)

	return withTraceID(mux)
}
