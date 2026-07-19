package http

import "context"

// userIDContextKey is an unexported key type: using a struct{} type (not a bare
// string) prevents collisions with keys set by other packages in the same
// context — the standard Go idiom for context values.
type userIDContextKey struct{}

// WithUserID stores the authenticated user id in the context. The auth
// middleware calls this after a token verifies; handlers read it back with
// UserIDFromContext instead of trusting the request body.
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDContextKey{}, userID)
}

// UserIDFromContext returns the authenticated user id, or "" if the request did
// not pass through the auth middleware (e.g. a public route).
func UserIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	if v, ok := ctx.Value(userIDContextKey{}).(string); ok {
		return v
	}

	return ""
}
