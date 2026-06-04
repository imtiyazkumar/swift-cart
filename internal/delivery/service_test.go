package delivery

import (
	"context"
	"testing"
)

type fakeDeliveryRepo struct {
	partnerID  string
	status     string
	location   LocationUpdate
	assignment *Assignment
}

func (f *fakeDeliveryRepo) UpsertAvailability(ctx context.Context, partnerID, status string) error {
	f.partnerID = partnerID
	f.status = status
	return nil
}

func (f *fakeDeliveryRepo) SaveLocation(ctx context.Context, update LocationUpdate) error {
	f.location = update
	return nil
}

func (f *fakeDeliveryRepo) CreateAssignment(ctx context.Context, assignment *Assignment) error {
	f.assignment = assignment
	return nil
}

func (f *fakeDeliveryRepo) UpdateAssignmentStatus(ctx context.Context, orderID, partnerID, status string) error {
	return nil
}

func TestGoOnlineSetsPartnerOnline(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	svc := NewService(repo)

	if err := svc.GoOnline(context.Background(), "partner-1"); err != nil {
		t.Fatalf("go online failed: %v", err)
	}
	if repo.partnerID != "partner-1" || repo.status != PartnerOnline {
		t.Fatalf("unexpected availability update: %s %s", repo.partnerID, repo.status)
	}
}

func TestUpdateLocationDefaultsRecordedAt(t *testing.T) {
	repo := &fakeDeliveryRepo{}
	svc := NewService(repo)

	if err := svc.UpdateLocation(context.Background(), LocationUpdate{PartnerID: "partner-1", Latitude: 19.07, Longitude: 72.87}); err != nil {
		t.Fatalf("update location failed: %v", err)
	}
	if repo.location.RecordedAt.IsZero() {
		t.Fatal("expected recorded_at to be defaulted")
	}
}

func TestAssignOrderRequiresIDs(t *testing.T) {
	svc := NewService(&fakeDeliveryRepo{})

	if _, err := svc.AssignOrder(context.Background(), "", "partner-1"); err == nil {
		t.Fatal("expected id validation error")
	}
}
