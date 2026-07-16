// Package usecase holds the order business logic. It orchestrates entities and
// the repo interface; it knows nothing about HTTP or SQL.
package usecase

import (
	"context"
	"fmt"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
)

// CreateOrderCommand is the input of Create — a controller-agnostic shape, so
// the usecase does not depend on any HTTP DTO.
type CreateOrderCommand struct {
	UserID         string
	IdempotencyKey string
	Items          []entity.OrderItem
}

// OrderUseCase implements the order business logic.
type OrderUseCase struct {
	repo OrderRepo
}

// NewOrderUseCase wires the dependency in through the constructor (no globals).
func NewOrderUseCase(repo OrderRepo) *OrderUseCase {
	return &OrderUseCase{repo: repo}
}

// Create validates the command into an Order (invariants live in the entity),
// then stores it idempotently.
//
// created=false means this was a duplicate request carrying an idempotency key
// that was already used: the caller gets back the original order rather than a
// second one.
func (uc *OrderUseCase) Create(ctx context.Context, cmd CreateOrderCommand) (*entity.Order, bool, error) {
	order, err := entity.NewOrder(cmd.UserID, cmd.IdempotencyKey, cmd.Items)
	if err != nil {
		return nil, false, fmt.Errorf("OrderUseCase.Create: %w", err)
	}

	stored, created, err := uc.repo.Create(ctx, order)
	if err != nil {
		return nil, false, fmt.Errorf("OrderUseCase.Create: %w", err)
	}

	return stored, created, nil
}

// GetByID loads one order. It propagates entity.ErrOrderNotFound so the
// controller can map it to 404.
func (uc *OrderUseCase) GetByID(ctx context.Context, id string) (*entity.Order, error) {
	order, err := uc.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("OrderUseCase.GetByID: %w", err)
	}

	return order, nil
}
