package entity

import (
	"fmt"
	"time"
)

// Status is the order lifecycle state. The values mirror the CHECK constraint
// in migrations/order/000001_orders.up.sql — the DB rejects anything else.
//
// Which transitions are legal is enforced here in the domain, not by the DB: a
// CHECK can validate one row's value but not what the previous value was.
type Status string

// Order lifecycle states, in the order they normally occur. See
// docs/system-architecture.md for the full state diagram including
// compensating transitions (PAYMENT_FAILED, INVENTORY_INSUFFICIENT -> CANCELLED).
const (
	StatusPending               Status = "PENDING"
	StatusInventoryReserved     Status = "INVENTORY_RESERVED"
	StatusPaymentProcessing     Status = "PAYMENT_PROCESSING"
	StatusCompleted             Status = "COMPLETED"
	StatusPaymentFailed         Status = "PAYMENT_FAILED"
	StatusInventoryInsufficient Status = "INVENTORY_INSUFFICIENT"
	StatusCancelled             Status = "CANCELLED"
)

// OrderItem is a line in an order.
type OrderItem struct {
	ID        string
	OrderID   string
	ProductID string
	Quantity  int
	Price     Money
}

// Subtotal is price x quantity, exact in minor units.
func (i OrderItem) Subtotal() Money { return i.Price.Mul(i.Quantity) }

// Order is the aggregate this service owns.
//
// ID/CreatedAt/UpdatedAt are assigned by the database (gen_random_uuid(),
// NOW()), so they are empty until the row is stored and read back.
type Order struct {
	ID             string
	UserID         string
	TotalAmount    Money
	Status         Status
	IdempotencyKey string
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Items          []OrderItem
}

// NewOrder builds a PENDING order and enforces the invariants that must hold
// before anything is written: an owner, an idempotency key, at least one item,
// sane quantities/prices, and a positive total (mirrors CHECK total_amount > 0).
//
// The total is computed here, never taken from the client: the client must not
// be able to decide what it pays.
func NewOrder(userID, idempotencyKey string, items []OrderItem) (*Order, error) {
	if userID == "" {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidOrder)
	}
	if idempotencyKey == "" {
		return nil, fmt.Errorf("%w: idempotency key is required", ErrInvalidOrder)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: at least one item is required", ErrInvalidOrder)
	}

	var total Money
	for _, item := range items {
		if item.ProductID == "" {
			return nil, fmt.Errorf("%w: product_id is required", ErrInvalidOrder)
		}
		if item.Quantity <= 0 {
			return nil, fmt.Errorf("%w: quantity must be > 0", ErrInvalidOrder)
		}
		if item.Price < 0 {
			return nil, fmt.Errorf("%w: price must be >= 0", ErrInvalidOrder)
		}
		total += item.Subtotal()
	}

	if total <= 0 {
		return nil, fmt.Errorf("%w: total must be > 0", ErrInvalidOrder)
	}

	return &Order{
		UserID:         userID,
		TotalAmount:    total,
		Status:         StatusPending,
		IdempotencyKey: idempotencyKey,
		Items:          items,
	}, nil
}
