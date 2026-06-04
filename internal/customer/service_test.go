package customer

import (
	"context"
	"testing"
)

type fakeCustomerRepo struct {
	customer *Customer
	deleted  string
}

func (f *fakeCustomerRepo) CreateCustomer(ctx context.Context, c *Customer) (string, error) {
	f.customer = c
	return "customer-1", nil
}

func (f *fakeCustomerRepo) GetCustomerByID(ctx context.Context, id string) (*Customer, error) {
	return f.customer, nil
}

func (f *fakeCustomerRepo) ListCustomers(ctx context.Context) ([]*Customer, error) {
	return []*Customer{f.customer}, nil
}

func (f *fakeCustomerRepo) UpdateCustomer(ctx context.Context, c *Customer) error {
	f.customer = c
	return nil
}

func (f *fakeCustomerRepo) DeleteCustomer(ctx context.Context, id string) error {
	f.deleted = id
	return nil
}

func TestCreateCustomerRequiresNameAndEmail(t *testing.T) {
	svc := NewService(&fakeCustomerRepo{})

	if _, err := svc.CreateCustomer(context.Background(), &Customer{Name: "Aisha"}); err == nil {
		t.Fatal("expected missing email error")
	}
}

func TestCreateCustomerSetsIDAndTimestamps(t *testing.T) {
	svc := NewService(&fakeCustomerRepo{})

	created, err := svc.CreateCustomer(context.Background(), &Customer{Name: "Aisha", Email: "aisha@example.test"})
	if err != nil {
		t.Fatalf("create customer failed: %v", err)
	}
	if created.ID != "customer-1" {
		t.Fatalf("expected repository id, got %q", created.ID)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatal("expected timestamps to be set")
	}
}

func TestDeleteCustomerRequiresID(t *testing.T) {
	svc := NewService(&fakeCustomerRepo{})

	if err := svc.DeleteCustomer(context.Background(), ""); err == nil {
		t.Fatal("expected id validation error")
	}
}
