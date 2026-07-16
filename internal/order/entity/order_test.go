package entity_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
)

func TestParseMoney(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    entity.Money
		wantErr bool
	}{
		{name: "two decimals", input: "10.50", want: 1050},
		{name: "one decimal is padded", input: "10.5", want: 1050},
		{name: "no decimals", input: "10", want: 1000},
		{name: "zero", input: "0.00", want: 0},
		{name: "surrounding spaces", input: " 7.25 ", want: 725},
		{name: "leading dot", input: ".99", want: 99},
		{name: "explicit plus", input: "+3.00", want: 300},
		{name: "negative", input: "-3.01", want: -301},
		{name: "cent precision is exact", input: "0.10", want: 10},
		{name: "three decimals rejected", input: "10.555", wantErr: true},
		{name: "empty rejected", input: "", wantErr: true},
		{name: "letters rejected", input: "abc", wantErr: true},
		{name: "mixed rejected", input: "10.5a", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := entity.ParseMoney(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseMoney(%q) = %v, want error", tt.input, got)
				}
				if !errors.Is(err, entity.ErrInvalidMoney) {
					t.Fatalf("error = %v, want ErrInvalidMoney", err)
				}

				return
			}
			if err != nil {
				t.Fatalf("ParseMoney(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseMoney(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestMoney_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		money entity.Money
		want  string
	}{
		{name: "whole", money: 1000, want: "10.00"},
		{name: "cents padded", money: 1005, want: "10.05"},
		{name: "zero", money: 0, want: "0.00"},
		{name: "sub unit", money: 9, want: "0.09"},
		{name: "negative", money: -1250, want: "-12.50"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.money.String(); got != tt.want {
				t.Fatalf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Round-tripping through JSON must stay exact — the whole reason Money is not a
// float. A JSON number and a JSON string must parse identically.
func TestMoney_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	type payload struct {
		Price entity.Money `json:"price"`
	}

	tests := []struct {
		name string
		raw  string
		want entity.Money
	}{
		{name: "string form", raw: `{"price":"10.10"}`, want: 1010},
		{name: "number form", raw: `{"price":10.10}`, want: 1010},
		{name: "integer form", raw: `{"price":3}`, want: 300},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got payload
			if err := json.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", tt.raw, err)
			}
			if got.Price != tt.want {
				t.Fatalf("price = %d, want %d", got.Price, tt.want)
			}

			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if want := `{"price":"` + tt.want.String() + `"}`; string(encoded) != want {
				t.Fatalf("Marshal = %s, want %s", encoded, want)
			}
		})
	}
}

// Summing cents must not drift the way float addition would (0.1 + 0.2).
func TestMoney_SumIsExact(t *testing.T) {
	t.Parallel()

	a, err := entity.ParseMoney("0.10")
	if err != nil {
		t.Fatalf("ParseMoney: %v", err)
	}
	b, err := entity.ParseMoney("0.20")
	if err != nil {
		t.Fatalf("ParseMoney: %v", err)
	}

	if got := (a + b).String(); got != "0.30" {
		t.Fatalf("0.10 + 0.20 = %s, want 0.30", got)
	}
}

func TestNewOrder(t *testing.T) {
	t.Parallel()

	const (
		userID = "11111111-1111-1111-1111-111111111111"
		key    = "key-1"
	)
	validItem := entity.OrderItem{ProductID: "22222222-2222-2222-2222-222222222222", Quantity: 2, Price: 1000}

	tests := []struct {
		name      string
		userID    string
		key       string
		items     []entity.OrderItem
		wantTotal entity.Money
		wantErr   bool
	}{
		{name: "valid single item", userID: userID, key: key, items: []entity.OrderItem{validItem}, wantTotal: 2000},
		{
			name:   "valid multiple items",
			userID: userID, key: key,
			items: []entity.OrderItem{
				validItem,
				{ProductID: "33333333-3333-3333-3333-333333333333", Quantity: 3, Price: 550},
			},
			wantTotal: 3650,
		},
		{name: "missing user", userID: "", key: key, items: []entity.OrderItem{validItem}, wantErr: true},
		{name: "missing idempotency key", userID: userID, key: "", items: []entity.OrderItem{validItem}, wantErr: true},
		{name: "no items", userID: userID, key: key, items: nil, wantErr: true},
		{
			name:   "zero quantity",
			userID: userID, key: key,
			items:   []entity.OrderItem{{ProductID: validItem.ProductID, Quantity: 0, Price: 1000}},
			wantErr: true,
		},
		{
			name:   "negative price",
			userID: userID, key: key,
			items:   []entity.OrderItem{{ProductID: validItem.ProductID, Quantity: 1, Price: -1}},
			wantErr: true,
		},
		{
			name:   "zero total rejected by invariant",
			userID: userID, key: key,
			items:   []entity.OrderItem{{ProductID: validItem.ProductID, Quantity: 1, Price: 0}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			order, err := entity.NewOrder(tt.userID, tt.key, tt.items)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, entity.ErrInvalidOrder) {
					t.Fatalf("error = %v, want ErrInvalidOrder", err)
				}

				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if order.TotalAmount != tt.wantTotal {
				t.Fatalf("total = %s, want %s", order.TotalAmount, tt.wantTotal)
			}
			if order.Status != entity.StatusPending {
				t.Fatalf("status = %q, want PENDING", order.Status)
			}
		})
	}
}
