package repo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
)

// loadOrderByID reads an order + its items. Split from the repo methods so both
// the pool path (GetByID) and the tx path (Create) share one implementation.
func loadOrderByID(ctx context.Context, q querier, id string) (*entity.Order, error) {
	order, err := scanOrder(q.QueryRow(ctx, selectOrderByID, id))
	if err != nil {
		return nil, fmt.Errorf("loadOrderByID: %w", err)
	}

	if err = attachItems(ctx, q, order); err != nil {
		return nil, fmt.Errorf("loadOrderByID: %w", err)
	}

	return order, nil
}

// loadOrderByKey is the idempotency replay path: the key already exists, so
// return whatever the first request stored.
func loadOrderByKey(ctx context.Context, q querier, key string) (*entity.Order, error) {
	order, err := scanOrder(q.QueryRow(ctx, selectOrderByIdempotencyKey, key))
	if err != nil {
		return nil, fmt.Errorf("loadOrderByKey: %w", err)
	}

	if err = attachItems(ctx, q, order); err != nil {
		return nil, fmt.Errorf("loadOrderByKey: %w", err)
	}

	return order, nil
}

// scanOrder maps one row. Money arrives as text (total_amount::text) and is
// parsed exactly; status is scanned as a plain string then converted, so the
// driver never has to know about the domain's named types.
func scanOrder(row pgx.Row) (*entity.Order, error) {
	var (
		order     entity.Order
		totalText string
		status    string
	)

	err := row.Scan(
		&order.ID,
		&order.UserID,
		&totalText,
		&status,
		&order.IdempotencyKey,
		&order.Version,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Translate the driver error into the domain's sentinel: callers above
		// must not have to import pgx to know an order is missing.
		return nil, entity.ErrOrderNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scanOrder: %w", err)
	}

	total, err := entity.ParseMoney(totalText)
	if err != nil {
		return nil, fmt.Errorf("scanOrder: total_amount: %w", err)
	}
	order.TotalAmount = total
	order.Status = entity.Status(status)

	return &order, nil
}

// attachItems loads the lines of an order.
func attachItems(ctx context.Context, q querier, order *entity.Order) error {
	rows, err := q.Query(ctx, selectOrderItems, order.ID)
	if err != nil {
		return fmt.Errorf("attachItems: query: %w", err)
	}
	defer rows.Close()

	items := make([]entity.OrderItem, 0, len(order.Items))
	for rows.Next() {
		var (
			item      entity.OrderItem
			priceText string
		)

		if err = rows.Scan(&item.ID, &item.OrderID, &item.ProductID, &item.Quantity, &priceText); err != nil {
			return fmt.Errorf("attachItems: scan: %w", err)
		}

		price, parseErr := entity.ParseMoney(priceText)
		if parseErr != nil {
			return fmt.Errorf("attachItems: price: %w", parseErr)
		}
		item.Price = price

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return fmt.Errorf("attachItems: rows: %w", err)
	}
	order.Items = items

	return nil
}
