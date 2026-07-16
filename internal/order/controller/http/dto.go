package http

import (
	"time"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
)

// DTOs are deliberately separate types from the entities: the wire format is a
// contract with clients and must be free to evolve independently of the domain.
// It also prevents accidentally exposing internal fields.

// createOrderRequest is the POST /orders body.
//
// user_id is temporary: it belongs in the JWT and will be read from the request
// context once auth middleware exists. A real client must not be able to place
// an order on someone else's behalf.
type createOrderRequest struct {
	UserID string            `json:"user_id" validate:"required,uuid"`
	Items  []createOrderItem `json:"items"   validate:"required,min=1,dive"`
}

type createOrderItem struct {
	ProductID string       `json:"product_id" validate:"required,uuid"`
	Quantity  int          `json:"quantity"   validate:"required,gt=0"`
	Price     entity.Money `json:"price"      validate:"gte=0"`
}

// toEntityItems maps the DTO to domain items. The total is NOT taken from the
// request — the entity computes it (see entity.NewOrder).
func (r createOrderRequest) toEntityItems() []entity.OrderItem {
	items := make([]entity.OrderItem, 0, len(r.Items))
	for _, item := range r.Items {
		items = append(items, entity.OrderItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		})
	}

	return items
}

// orderResponse is what clients read back. Money marshals as exact decimal text.
type orderResponse struct {
	ID             string              `json:"id"`
	UserID         string              `json:"user_id"`
	TotalAmount    entity.Money        `json:"total_amount"`
	Status         string              `json:"status"`
	IdempotencyKey string              `json:"idempotency_key"`
	Version        int                 `json:"version"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	Items          []orderItemResponse `json:"items"`
}

type orderItemResponse struct {
	ID        string       `json:"id"`
	ProductID string       `json:"product_id"`
	Quantity  int          `json:"quantity"`
	Price     entity.Money `json:"price"`
	Subtotal  entity.Money `json:"subtotal"`
}

func newOrderResponse(order *entity.Order) orderResponse {
	items := make([]orderItemResponse, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, orderItemResponse{
			ID:        item.ID,
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
			Subtotal:  item.Subtotal(),
		})
	}

	return orderResponse{
		ID:             order.ID,
		UserID:         order.UserID,
		TotalAmount:    order.TotalAmount,
		Status:         string(order.Status),
		IdempotencyKey: order.IdempotencyKey,
		Version:        order.Version,
		CreatedAt:      order.CreatedAt,
		UpdatedAt:      order.UpdatedAt,
		Items:          items,
	}
}
