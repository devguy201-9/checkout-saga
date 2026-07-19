// Package repo implements the data access declared by internal/order/usecase.
package repo

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
	"github.com/devguy201-9/checkout-saga/pkg/postgres"
)

// Money and UUID cross the boundary as text with an explicit cast ($n::numeric,
// $n::uuid, id::text). Reason: exactness. A numeric read as a Go float would
// drift; text is the lossless representation both sides agree on, and it keeps
// the repo free of extra codec dependencies.
const (
	// ON CONFLICT DO NOTHING + RETURNING yields no row on a duplicate key —
	// that is the signal for "this idempotency key was already used".
	insertOrder = `
		INSERT INTO orders (user_id, total_amount, status, idempotency_key)
		VALUES ($1::uuid, $2::numeric, $3, $4)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id::text`

	insertOrderItem = `
		INSERT INTO order_items (order_id, product_id, quantity, price)
		VALUES ($1::uuid, $2::uuid, $3, $4::numeric)`

	selectOrderByID = `
		SELECT id::text, user_id::text, total_amount::text, status,
		       idempotency_key, version, created_at, updated_at
		FROM orders
		WHERE id = $1::uuid`

	selectOrderByIdempotencyKey = `
		SELECT id::text, user_id::text, total_amount::text, status,
		       idempotency_key, version, created_at, updated_at
		FROM orders
		WHERE idempotency_key = $1`

	selectOrderItems = `
		SELECT id::text, order_id::text, product_id::text, quantity, price::text
		FROM order_items
		WHERE order_id = $1::uuid
		ORDER BY id`
)

// querier is the slice of pgx that the read helpers need, so they work against
// either the pool or a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// OrderRepo implements usecase.OrderRepo on PostgreSQL.
type OrderRepo struct {
	pool *pgxpool.Pool
}

// NewOrderRepo takes the wrapper rather than the raw pool so the composition
// root keeps passing one object around.
func NewOrderRepo(pg *postgres.Postgres) *OrderRepo {
	return &OrderRepo{pool: pg.Pool}
}

// Create inserts the order and its items in one transaction.
//
// Concurrency: two identical requests race here. The UNIQUE index on
// idempotency_key serialises them — the loser's INSERT conflicts, DO NOTHING
// returns no row, and it reads the winner's order instead. No check-then-insert
// gap, so no duplicate order is ever created.
func (r *OrderRepo) Create(ctx context.Context, order *entity.Order) (*entity.Order, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("OrderRepo.Create: begin: %w", err)
	}
	// Rollback is a no-op once the tx is committed; this guards every early return.
	// Any error here (beyond "already committed") is logged, never swallowed silently.
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			fmt.Fprintf(os.Stderr, "OrderRepo.Create: rollback: %v\n", rbErr)
		}
	}()

	var orderID string
	err = tx.QueryRow(
		ctx, insertOrder,
		order.UserID, order.TotalAmount.String(), string(order.Status), order.IdempotencyKey,
	).Scan(&orderID)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Duplicate idempotency key: return the order the first request stored.
		existing, getErr := loadOrderByKey(ctx, tx, order.IdempotencyKey)
		if getErr != nil {
			return nil, false, fmt.Errorf("OrderRepo.Create: load existing: %w", getErr)
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return nil, false, fmt.Errorf("OrderRepo.Create: commit: %w", commitErr)
		}

		return existing, false, nil

	case err != nil:
		return nil, false, fmt.Errorf("OrderRepo.Create: insert order: %w", err)
	}

	if err = insertItems(ctx, tx, orderID, order.Items); err != nil {
		return nil, false, fmt.Errorf("OrderRepo.Create: %w", err)
	}

	// Read back so the caller gets DB-assigned values (id, status, timestamps).
	stored, err := loadOrderByID(ctx, tx, orderID)
	if err != nil {
		return nil, false, fmt.Errorf("OrderRepo.Create: reload: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, false, fmt.Errorf("OrderRepo.Create: commit: %w", err)
	}

	return stored, true, nil
}

// GetByID loads one order with its items.
func (r *OrderRepo) GetByID(ctx context.Context, id string) (*entity.Order, error) {
	order, err := loadOrderByID(ctx, r.pool, id)
	if err != nil {
		return nil, fmt.Errorf("OrderRepo.GetByID: %w", err)
	}

	return order, nil
}

// insertItems sends all line inserts in one batch — one network round trip
// instead of N (the N+1 rule in docs/code-standards.md).
func insertItems(ctx context.Context, tx pgx.Tx, orderID string, items []entity.OrderItem) error {
	batch := &pgx.Batch{}
	for _, item := range items {
		batch.Queue(insertOrderItem, orderID, item.ProductID, item.Quantity, item.Price.String())
	}

	results := tx.SendBatch(ctx, batch)
	for range items {
		if _, err := results.Exec(); err != nil {
			if closeErr := results.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "insertItems: close batch after exec error: %v\n", closeErr)
			}

			return fmt.Errorf("insertItems: exec: %w", err)
		}
	}

	// The batch must be closed before the tx is used again.
	if err := results.Close(); err != nil {
		return fmt.Errorf("insertItems: close batch: %w", err)
	}

	return nil
}
