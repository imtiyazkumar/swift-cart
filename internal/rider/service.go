package rider

import (
    "context"
    "errors"
)

type Service struct {
    repo Repository
}

func NewService(r Repository) *Service {
    return &Service{repo: r}
}

func (s *Service) GetProfile(ctx context.Context, userID string) (*RiderProfile, error) {
    if userID == "" {
        return nil, errors.New("user ID required")
    }
    return s.repo.GetRiderByUserID(ctx, userID)
}

func (s *Service) UpdateStatus(ctx context.Context, partnerID, status string) error {
    if partnerID == "" || status == "" {
        return errors.New("partnerID and status required")
    }
    return s.repo.UpdateStatus(ctx, partnerID, status)
}

func (s *Service) RecordLocation(ctx context.Context, partnerID string, lat, lng float64) error {
    if partnerID == "" {
        return errors.New("partnerID required")
    }
    return s.repo.RecordLocation(ctx, partnerID, lat, lng)
}

func (s *Service) ListAvailableJobs(ctx context.Context) ([]*DeliveryJob, error) {
    return s.repo.ListAvailableJobs(ctx)
}

func (s *Service) AcceptJob(ctx context.Context, partnerID, orderID string) error {
    if partnerID == "" || orderID == "" {
        return errors.New("partnerID and orderID required")
    }
    return s.repo.AcceptJob(ctx, partnerID, orderID)
}

func (s *Service) UpdateJobStatus(ctx context.Context, partnerID, orderID, status string) error {
    if partnerID == "" || orderID == "" || status == "" {
        return errors.New("partnerID, orderID, and status required")
    }
    return s.repo.UpdateJobStatus(ctx, partnerID, orderID, status)
}

func (s *Service) GetEarningsSummary(ctx context.Context, partnerID string) (*RiderEarningsSummary, error) {
    if partnerID == "" {
        return nil, errors.New("partnerID required")
    }
    return s.repo.GetEarningsSummary(ctx, partnerID)
}
