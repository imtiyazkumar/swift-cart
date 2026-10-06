package rider

import (
	"context"
	"testing"
)

type fakeRiderRepo struct{}

func (f *fakeRiderRepo) GetRiderByUserID(ctx context.Context, userID string) (*RiderProfile, error) {
	return &RiderProfile{ID: "rider-1", UserID: userID, Name: "Tariq Rider", Status: "ONLINE"}, nil
}

func (f *fakeRiderRepo) UpdateStatus(ctx context.Context, partnerID, status string) error {
	return nil
}

func (f *fakeRiderRepo) RecordLocation(ctx context.Context, partnerID string, lat, lng float64) error {
	return nil
}

func (f *fakeRiderRepo) ListAvailableJobs(ctx context.Context) ([]*DeliveryJob, error) {
	return []*DeliveryJob{{OrderID: "order-1", MerchantName: "Ahdoos"}}, nil
}

func (f *fakeRiderRepo) AcceptJob(ctx context.Context, partnerID, orderID string) error {
	return nil
}

func (f *fakeRiderRepo) UpdateJobStatus(ctx context.Context, partnerID, orderID, status string) error {
	return nil
}

func (f *fakeRiderRepo) GetEarningsSummary(ctx context.Context, partnerID string) (*RiderEarningsSummary, error) {
	return &RiderEarningsSummary{TotalDeliveries: 10, TotalEarnings: 4000}, nil
}

func TestRiderServiceGetProfile(t *testing.T) {
	svc := NewService(&fakeRiderRepo{})
	profile, err := svc.GetProfile(context.Background(), "user-rider-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.Name != "Tariq Rider" {
		t.Fatalf("expected Tariq Rider, got %s", profile.Name)
	}
}

func TestRiderServiceValidation(t *testing.T) {
	svc := NewService(&fakeRiderRepo{})
	if err := svc.UpdateStatus(context.Background(), "", "ONLINE"); err == nil {
		t.Fatal("expected validation error for empty partnerID")
	}
}
