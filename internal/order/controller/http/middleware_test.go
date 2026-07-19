package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/devguy201-9/checkout-saga/pkg/jwt"
	"github.com/devguy201-9/checkout-saga/pkg/logger"
)

const (
	_mwSecret = "middleware-test-secret-32-chars-x!"
	_mwUserID = "11111111-1111-1111-1111-111111111111"
)

// authEnvelope is the minimal shape needed to read back the error code.
type authEnvelope struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// newBearer builds an "Authorization: Bearer <token>" value from a manager.
func newBearer(t *testing.T, mgr *jwt.Manager, expiry time.Duration) string {
	t.Helper()

	m := jwt.NewManager(_mwSecret, expiry)
	if mgr != nil {
		m = mgr
	}

	token, err := m.Generate(_mwUserID, time.Now())
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return "Bearer " + token
}

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()

	log := logger.New("error", false) // quiet: only errors surface, none expected
	mgr := jwt.NewManager(_mwSecret, time.Hour)

	wrongMgr := jwt.NewManager("a-totally-different-secret-32-chr!", time.Hour)

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
		wantCode   string // "" => success expected
	}{
		{
			name:       "valid token passes and injects user id",
			authHeader: newBearer(t, mgr, time.Hour),
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing header is 401 missing_token",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "missing_token",
		},
		{
			name:       "no bearer prefix is 401 missing_token",
			authHeader: "Token abc.def.ghi",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "missing_token",
		},
		{
			name:       "bearer prefix with empty token is 401 invalid_token",
			authHeader: "Bearer ",
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_token",
		},
		{
			name:       "expired token is 401 invalid_token",
			authHeader: newBearer(t, jwt.NewManager(_mwSecret, -time.Minute), 0),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_token",
		},
		{
			name:       "bad signature is 401 invalid_token",
			authHeader: newBearer(t, wrongMgr, time.Hour),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "invalid_token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotUserID string
			var reached bool
			stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reached = true
				gotUserID = UserIDFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			handler := authMiddleware(mgr, log)(stub)

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/orders", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: want %d, got %d (body=%s)", tc.wantStatus, rec.Code, rec.Body.String())
			}

			if tc.wantCode == "" {
				// Success path: the stub must run and see the token's subject.
				if !reached {
					t.Fatal("valid token did not reach the wrapped handler")
				}
				if gotUserID != _mwUserID {
					t.Fatalf("user_id: want %q, got %q", _mwUserID, gotUserID)
				}

				return
			}

			// Failure path: the stub must NOT run, and the envelope carries the code.
			if reached {
				t.Fatal("wrapped handler ran despite auth failure")
			}

			var env authEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if env.Error == nil || env.Error.Code != tc.wantCode {
				t.Fatalf("error code: want %q, got %+v", tc.wantCode, env.Error)
			}
		})
	}
}
