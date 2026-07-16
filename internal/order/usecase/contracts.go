package usecase

import (
	"context"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
)

// OrderRepo is defined here, on the consumer side (the Go idiom): the usecase
// declares what it needs, and internal/order/repo provides an implementation.
// That inverts the dependency — usecase never imports repo — and makes the
// usecase testable with a fake.
type OrderRepo interface {
	// Create stores the order idempotently.
	//
	// created=false means the idempotency key already existed and the returned
	// order is the one stored by the first request (a replay, not an error).
	// The dedup itself is atomic in the database, not a check-then-insert here
	// (see docs/adr/0005-idempotency-insert-on-conflict.md).
	Create(ctx context.Context, order *entity.Order) (stored *entity.Order, created bool, err error)

	// GetByID returns entity.ErrOrderNotFound when the id does not exist.
	GetByID(ctx context.Context, id string) (*entity.Order, error)
}
