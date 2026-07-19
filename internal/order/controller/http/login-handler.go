package http

import (
	"time"

	nethttp "net/http"

	"github.com/go-playground/validator/v10"
	"go.uber.org/zap"

	"github.com/devguy201-9/checkout-saga/pkg/jwt"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

// loginHandler issues tokens.
//
// DEV MOCK: this endpoint takes a user_id and hands back a signed token WITHOUT
// any password check. It exists so the auth flow is demoable end-to-end; a real
// issuer (password/OAuth + a user store) is explicitly out of scope for this
// chunk (see docs/order-jwt-auth.md).
type loginHandler struct {
	jwtMgr   *jwt.Manager
	log      logger.Logger
	validate *validator.Validate
}

func newLoginHandler(jwtMgr *jwt.Manager, log logger.Logger) *loginHandler {
	return &loginHandler{
		jwtMgr:   jwtMgr,
		log:      log,
		validate: validator.New(validator.WithRequiredStructEnabled()),
	}
}

// loginRequest is the POST /login body: just the identity to mint a token for.
type loginRequest struct {
	UserID string `json:"user_id" validate:"required,uuid"`
}

// loginResponse carries the freshly minted token under the standard data envelope.
type loginResponse struct {
	Token string `json:"token"`
}

// issue handles POST /login.
func (h *loginHandler) issue(w nethttp.ResponseWriter, r *nethttp.Request) {
	ctx := r.Context()

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, h.log, nethttp.StatusBadRequest, "invalid_body", err.Error())

		return
	}

	if err := h.validate.Struct(req); err != nil {
		writeError(w, h.log, nethttp.StatusBadRequest, "validation_failed", err.Error())

		return
	}

	// time.Now is read here (the composition edge), not inside pkg/jwt, so the
	// package stays clock-free and unit-testable.
	token, err := h.jwtMgr.Generate(req.UserID, time.Now())
	if err != nil {
		h.log.Error(
			"issue token failed",
			zap.Error(err),
			zap.String("trace_id", logger.TraceIDFromContext(ctx)),
		)
		writeError(w, h.log, nethttp.StatusInternalServerError, "internal_error", "internal server error")

		return
	}

	writeJSON(w, h.log, nethttp.StatusOK, loginResponse{Token: token})
}
