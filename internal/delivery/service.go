package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Repository interface {
	UpsertAvailability(ctx context.Context, partnerID, status string) error
	SaveLocation(ctx context.Context, update LocationUpdate) error
	CreateAssignment(ctx context.Context, assignment *Assignment) error
	UpdateAssignmentStatus(ctx context.Context, orderID, partnerID, status string) error
}

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) GoOnline(ctx context.Context, partnerID string) error {
	if partnerID == "" {
		return errors.New("partner_id is required")
	}
	return s.repo.UpsertAvailability(ctx, partnerID, PartnerOnline)
}

func (s *Service) GoOffline(ctx context.Context, partnerID string) error {
	if partnerID == "" {
		return errors.New("partner_id is required")
	}
	return s.repo.UpsertAvailability(ctx, partnerID, PartnerOffline)
}

func (s *Service) UpdateLocation(ctx context.Context, update LocationUpdate) error {
	if update.PartnerID == "" {
		return errors.New("partner_id is required")
	}
	if update.RecordedAt.IsZero() {
		update.RecordedAt = time.Now().UTC()
	}
	return s.repo.SaveLocation(ctx, update)
}

func (s *Service) AssignOrder(ctx context.Context, orderID, partnerID string) (*Assignment, error) {
	if orderID == "" || partnerID == "" {
		return nil, errors.New("order_id and partner_id are required")
	}
	assignment := &Assignment{ID: uuid.NewString(), OrderID: orderID, PartnerID: partnerID, Status: "ASSIGNED"}
	if err := s.repo.CreateAssignment(ctx, assignment); err != nil {
		return nil, err
	}
	return assignment, nil
}
