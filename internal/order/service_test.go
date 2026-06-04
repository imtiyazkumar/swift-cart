package order

import (
	"context"
	"testing"
)

type fakeRepo struct {
	order  *Order
	status string
}

func (f *fakeRepo) Create(ctx context.Context, o *Order) error {
	f.order = o
	return nil
}

func (f *fakeRepo) GetByID(ctx context.Context, id string) (*Order, error) { return f.order, nil }
func (f *fakeRepo) List(ctx context.Context) ([]*Order, error)             { return []*Order{f.order}, nil }
func (f *fakeRepo) Delete(ctx context.Context, id string) error            { return nil }
func (f *fakeRepo) UpdateStatus(ctx context.Context, id string, status string) error {
	f.status = status
	return nil
}

func TestCreateOrderDefaultsToCreated(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	created, err := svc.CreateOrder(context.Background(), &Order{MerchantID: "m1", CustomerID: "c1"})
	if err != nil {
		t.Fatalf("create order failed: %v", err)
	}
	if created.Status != "CREATED" {
		t.Fatalf("expected CREATED status, got %q", created.Status)
	}
}

func TestUpdateOrderStatusRejectsUnknownStatus(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)

	if err := svc.UpdateOrderStatus(context.Background(), "o1", "PENDING"); err == nil {
		t.Fatal("expected invalid status error")
	}
}
