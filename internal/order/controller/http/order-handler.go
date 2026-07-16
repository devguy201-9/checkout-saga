package http

import (
	"context"
	"errors"
	nethttp "net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
	"github.com/devguy201-9/checkout-saga/internal/order/usecase"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// _idempotencyHeader is the client-generated key that makes POST /orders safe
// to retry (see docs/adr/0005-idempotency-insert-on-conflict.md).
const _idempotencyHeader = "Idempotency-Key"

type orderHandler struct {
	orderUseCase *usecase.OrderUseCase
	log          logger.Logger
	validate     *validator.Validate
}

func newOrderHandler(orderUseCase *usecase.OrderUseCase, log logger.Logger) *orderHandler {
	return &orderHandler{
		orderUseCase: orderUseCase,
		log:          log,
		// Built once per handler, not a package-level global (no global state).
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// create handles POST /orders.
//
// 201 when the order is new, 200 when the Idempotency-Key was already used and
// the original order is returned. The distinction is honest about what happened
// while both are success — a retrying client sees the same order either way.
func (h *orderHandler) create(w nethttp.ResponseWriter, r *nethttp.Request) {
	ctx := r.Context()

	key := strings.TrimSpace(r.Header.Get(_idempotencyHeader))
	if key == "" {
		writeError(w, h.log, nethttp.StatusBadRequest,
			"missing_idempotency_key", _idempotencyHeader+" header is required")

		return
	}

	var req createOrderRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, h.log, nethttp.StatusBadRequest, "invalid_body", err.Error())

		return
	}

	if err := h.validate.Struct(req); err != nil {
		writeError(w, h.log, nethttp.StatusBadRequest, "validation_failed", err.Error())

		return
	}

	order, created, err := h.orderUseCase.Create(ctx, usecase.CreateOrderCommand{
		UserID:         req.UserID,
		IdempotencyKey: key,
		Items:          req.toEntityItems(),
	})
	if err != nil {
		h.writeUseCaseError(ctx, w, "create order", err)

		return
	}

	status := nethttp.StatusOK
	if created {
		status = nethttp.StatusCreated
	}

	h.log.Info("order create handled",
		zap.String("order_id", order.ID),
		zap.Bool("created", created),
		zap.String("trace_id", logger.TraceIDFromContext(ctx)),
	)
	writeJSON(w, h.log, status, newOrderResponse(order))
}

// getByID handles GET /orders/{id}.
func (h *orderHandler) getByID(w nethttp.ResponseWriter, r *nethttp.Request) {
	ctx := r.Context()

	id := r.PathValue("id")
	// Validate the shape before hitting the DB: a malformed uuid is a 400 from
	// the client, not a 500 from a failed cast.
	if err := h.validate.Var(id, "required,uuid"); err != nil {
		writeError(w, h.log, nethttp.StatusBadRequest, "invalid_order_id", "order id must be a uuid")

		return
	}

	order, err := h.orderUseCase.GetByID(ctx, id)
	if err != nil {
		h.writeUseCaseError(ctx, w, "get order", err)

		return
	}

	writeJSON(w, h.log, nethttp.StatusOK, newOrderResponse(order))
}

// writeUseCaseError maps domain sentinels to status codes. This is the only
// place that knows both worlds; the usecase never mentions HTTP.
func (h *orderHandler) writeUseCaseError(ctx context.Context, w nethttp.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, entity.ErrOrderNotFound):
		writeError(w, h.log, nethttp.StatusNotFound, "order_not_found", "order not found")

	case errors.Is(err, entity.ErrInvalidOrder), errors.Is(err, entity.ErrInvalidMoney):
		writeError(w, h.log, nethttp.StatusBadRequest, "invalid_order", err.Error())

	default:
		// Unexpected: log the detail, return a generic message. Internal errors
		// can leak schema/infrastructure details to a caller.
		h.log.Error(op+" failed",
			zap.Error(err),
			zap.String("trace_id", logger.TraceIDFromContext(ctx)),
		)
		writeError(w, h.log, nethttp.StatusInternalServerError, "internal_error", "internal server error")
	}
}
