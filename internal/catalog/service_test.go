package catalog

import (
	"context"
	"testing"
)

type fakeCatalogRepo struct {
	item *Item
}

func (f *fakeCatalogRepo) CreateCategory(ctx context.Context, category *Category) error { return nil }
func (f *fakeCatalogRepo) CreateItem(ctx context.Context, item *Item) error {
	f.item = item
	return nil
}
func (f *fakeCatalogRepo) GetItem(ctx context.Context, id string) (*Item, error) { return f.item, nil }
func (f *fakeCatalogRepo) ListMerchantItems(ctx context.Context, merchantID string) ([]*Item, error) {
	return []*Item{f.item}, nil
}
func (f *fakeCatalogRepo) SearchItems(ctx context.Context, query string, limit int) ([]*Item, error) {
	return []*Item{f.item}, nil
}
func (f *fakeCatalogRepo) UpdateItem(ctx context.Context, item *Item) error { return nil }

func TestCreateItemAppliesGenericDefaults(t *testing.T) {
	repo := &fakeCatalogRepo{}
	svc := NewService(repo)

	item, err := svc.CreateItem(context.Background(), &Item{
		MerchantID: "m1",
		CategoryID: "c1",
		Name:       "Organic Apples",
		BasePrice:  12000,
	})
	if err != nil {
		t.Fatalf("create item failed: %v", err)
	}
	if item.Currency != "INR" || item.ItemType != "STANDARD" || item.SubstitutionPolicy != "NONE" {
		t.Fatalf("generic defaults were not applied: %+v", item)
	}
}
