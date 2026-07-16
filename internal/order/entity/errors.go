// Package entity holds the Order domain: the aggregate (Order/OrderItem),
// value objects (Money, Status), invariants enforced at construction time, and
// the sentinel errors callers match against with errors.Is. Pure Go — no
// external dependencies (see docs/code-standards.md package import rules).
package entity

import "errors"

// Domain-level expected errors. Callers match with errors.Is and map them to a
// status code at the controller boundary — the domain never knows about HTTP.
var (
	// ErrOrderNotFound: no order with that id.
	ErrOrderNotFound = errors.New("order: not found")

	// ErrInvalidOrder: an invariant was violated (no items, total <= 0, ...).
	ErrInvalidOrder = errors.New("order: invalid")

	// ErrInvalidMoney: money text was not exact decimal (see money.go).
	ErrInvalidMoney = errors.New("order: invalid money format")

	// ErrInvalidTransition: an illegal move in the status state machine.
	// Used once the saga starts driving orders forward.
	ErrInvalidTransition = errors.New("order: invalid status transition")
)
