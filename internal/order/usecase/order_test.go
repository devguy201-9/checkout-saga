package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/devguy201-9/checkout-saga/internal/order/entity"
	"github.com/devguy201-9/checkout-saga/internal/order/usecase"
)

// fakeRepo is a hand-written stub. The usecase depends on the OrderRepo
// interface, so no database is needed to test the business logic.
type fakeRepo struct {
	createFn  func(ctx context.Context, o *entity.Order) (*entity.Order, bool, error)
	getByIDFn func(ctx context.Context, id string) (*entity.Order, error)

	createCalls int
	gotOrder    *entity.Order
}

func (f *fakeRepo) Create(ctx context.Context, order *entity.Order) (*entity.Order, bool, error) {
	f.createCalls++
	f.gotOrder = order

	return f.createFn(ctx, order)
}

func (f *fakeRepo) GetByID(ctx context.Context, id string) (*entity.Order, error) {
	return f.getByIDFn(ctx, id)
}

const (
	testUserID    = "11111111-1111-1111-1111-111111111111"
	testProductID = "22222222-2222-2222-2222-222222222222"
)

func validCommand() usecase.CreateOrderCommand {
	return usecase.CreateOrderCommand{
		UserID:         testUserID,
		IdempotencyKey: "key-1",
		Items: []entity.OrderItem{
			{ProductID: testProductID, Quantity: 2, Price: 1050},
		},
	}
}

func TestOrderUseCase_Create_StoresPendingOrderWithComputedTotal(t *testing.T) {
	t.Parallel()

	repo := &fakeRepo{
		createFn: func(_ context.Context, o *entity.Order) (*entity.Order, bool, error) {
			stored := *o
			stored.ID = "order-1"

			return &stored, true, nil
		},
	}
	uc := usecase.NewOrderUseCase(repo)

	order, created, err := uc.Create(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("created = false, want true for a fresh key")
	}
	// The total is computed from the items, never trusted from the caller.
	if order.TotalAmount != 2100 {
		t.Fatalf("total = %s, want 21.00", order.TotalAmount)
	}
	if repo.gotOrder.Status != entity.StatusPending {
		t.Fatalf("status = %q, want PENDING", repo.gotOrder.Status)
	}
}

// A replayed idempotency key is a success, not an error: the caller gets the
// original order back and created=false.
func TestOrderUseCase_Create_ReplayReturnsExistingOrder(t *testing.T) {
	t.Parallel()

	existing := &entity.Order{ID: "order-1", Status: entity.StatusPending, IdempotencyKey: "key-1"}
	repo := &fakeRepo{
		createFn: func(_ context.Context, _ *entity.Order) (*entity.Order, bool, error) {
			return existing, false, nil
		},
	}
	uc := usecase.NewOrderUseCase(repo)

	order, created, err := uc.Create(context.Background(), validCommand())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Fatal("created = true, want false for a duplicate key")
	}
	if order.ID != existing.ID {
		t.Fatalf("id = %q, want %q", order.ID, existing.ID)
	}
}

// Invalid commands must never reach the database.
func TestOrderUseCase_Create_RejectsInvalidCommandBeforeRepo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  usecase.CreateOrderCommand
	}{
		{name: "no items", cmd: usecase.CreateOrderCommand{UserID: testUserID, IdempotencyKey: "key-1"}},
		{
			name: "missing idempotency key",
			cmd: usecase.CreateOrderCommand{
				UserID: testUserID,
				Items:  []entity.OrderItem{{ProductID: testProductID, Quantity: 1, Price: 100}},
			},
		},
		{
			name: "zero total",
			cmd: usecase.CreateOrderCommand{
				UserID: testUserID, IdempotencyKey: "key-1",
				Items: []entity.OrderItem{{ProductID: testProductID, Quantity: 1, Price: 0}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &fakeRepo{
				createFn: func(_ context.Context, _ *entity.Order) (*entity.Order, bool, error) {
					t.Fatal("repo.Create must not be called for an invalid command")

					return nil, false, nil
				},
			}
			uc := usecase.NewOrderUseCase(repo)

			if _, _, err := uc.Create(context.Background(), tt.cmd); !errors.Is(err, entity.ErrInvalidOrder) {
				t.Fatalf("error = %v, want ErrInvalidOrder", err)
			}
			if repo.createCalls != 0 {
				t.Fatalf("repo.Create called %d times, want 0", repo.createCalls)
			}
		})
	}
}

func TestOrderUseCase_Create_PropagatesRepoError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("db down")
	repo := &fakeRepo{
		createFn: func(_ context.Context, _ *entity.Order) (*entity.Order, bool, error) {
			return nil, false, wantErr
		},
	}
	uc := usecase.NewOrderUseCase(repo)

	if _, _, err := uc.Create(context.Background(), validCommand()); !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
}

func TestOrderUseCase_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("returns order", func(t *testing.T) {
		t.Parallel()

		want := &entity.Order{ID: "order-1"}
		uc := usecase.NewOrderUseCase(&fakeRepo{
			getByIDFn: func(_ context.Context, _ string) (*entity.Order, error) { return want, nil },
		})

		got, err := uc.GetByID(context.Background(), "order-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != want.ID {
			t.Fatalf("id = %q, want %q", got.ID, want.ID)
		}
	})

	// The sentinel must survive wrapping so the controller can map it to 404.
	t.Run("propagates not found", func(t *testing.T) {
		t.Parallel()

		uc := usecase.NewOrderUseCase(&fakeRepo{
			getByIDFn: func(_ context.Context, _ string) (*entity.Order, error) {
				return nil, entity.ErrOrderNotFound
			},
		})

		if _, err := uc.GetByID(context.Background(), "missing"); !errors.Is(err, entity.ErrOrderNotFound) {
			t.Fatalf("error = %v, want ErrOrderNotFound", err)
		}
	})
}
