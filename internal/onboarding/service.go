package onboarding

import (
    "context"
    "encoding/json"
    "errors"
    "strings"

    "github.com/google/uuid"
    "github.com/imtiyazkumar/yember-core/internal/stations"
    "github.com/jackc/pgx/v5/pgxpool"
)

type Application struct {
    Latitude           *float64 `json:"latitude"`
    Longitude          *float64 `json:"longitude"`
    StationID          string   `json:"station_id"`
    Name               string   `json:"name"`
    Phone              string   `json:"phone"`
    Address            string   `json:"address"`
    Description        string   `json:"description"`
    LicenseNumber      string   `json:"license_number"`
    Cuisine            []string `json:"cuisine"`
    VehicleType        string   `json:"vehicle_type"`
    RegistrationNumber string   `json:"registration_number"`
    DocumentIDs        []string `json:"document_ids"`
}

func (a Application) Validate(kind string) error {
    if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.Phone) == "" || strings.TrimSpace(a.Address) == "" || strings.TrimSpace(a.LicenseNumber) == "" {
        return errors.New("name, phone, address and license number are required")
    }
    if len(a.Name) > 200 || len(a.Address) > 2000 || len(a.Description) > 5000 || len(a.Phone) > 30 || len(a.LicenseNumber) > 200 {
        return errors.New("application fields exceed their maximum length")
    }
    if kind == "restaurant" && (len(a.Cuisine) == 0 || strings.TrimSpace(a.Description) == "") {
        return errors.New("restaurant description and cuisine are required")
    }
    if kind == "delivery" && (a.VehicleType == "" || a.RegistrationNumber == "") {
        return errors.New("vehicle type and registration number required")
    }
    if len(a.DocumentIDs) == 0 || len(a.DocumentIDs) > 10 {
        return errors.New("one to ten verification documents required")
    }
    seen := map[string]bool{}
    for _, id := range a.DocumentIDs {
        if _, err := uuid.Parse(id); err != nil || seen[id] {
            return errors.New("invalid or duplicate document ID")
        }
        seen[id] = true
    }
    return nil
}

type Service struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) Submit(ctx context.Context, userID, kind string, a Application) (string, error) {
    if err := a.Validate(kind); err != nil {
        return "", err
    }
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return "", err
    }
    defer tx.Rollback(ctx)
    if err = stations.LockCoverage(ctx, tx); err != nil {
        return "", err
    }
    var stationID string
    if kind == "restaurant" {
        stationID, err = stations.Resolve(ctx, tx, a.Latitude, a.Longitude)
        if err != nil {
            return "", err
        }
    } else {
        if _, err = uuid.Parse(a.StationID); err != nil {
            return "", errors.New("select an active operational station")
        }
        var active bool
        if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.stations WHERE id=$1 AND active)`, a.StationID).Scan(&active); err != nil {
            return "", err
        }
        if !active {
            return "", errors.New("select an active operational station")
        }
        stationID = a.StationID
    }
    var email string
    if err = tx.QueryRow(ctx, `SELECT email FROM public.users WHERE id=$1 AND email_verified_at IS NOT NULL AND disabled_at IS NULL AND deleted_at IS NULL FOR UPDATE`, userID).Scan(&email); err != nil {
        return "", errors.New("verify your email before submitting an application")
    }
    var count int
    if err = tx.QueryRow(ctx, `SELECT count(*) FROM public.private_documents WHERE user_id=$1 AND id=ANY($2::uuid[])`, userID, a.DocumentIDs).Scan(&count); err != nil {
        return "", err
    }
    if count != len(a.DocumentIDs) {
        return "", errors.New("documents must belong to the applicant")
    }
    var existing bool
    if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.partner_applications WHERE user_id=$1 AND kind=$2)`, userID, kind).Scan(&existing); err != nil {
        return "", err
    }
    if existing {
        return "", errors.New("an application already exists; contact support for resubmission")
    }
    var id string
    if kind == "restaurant" {
        err = tx.QueryRow(ctx, `INSERT INTO public.merchants(name,email,phone,owner_id,merchant_type,onboarding_status,approval_status,is_active,tags,station_id,latitude,longitude) VALUES($1,$2,$3,$4,'RESTAURANT','SUBMITTED','PENDING',false,$5,$6,$7,$8) RETURNING id`, a.Name, email, a.Phone, userID, a.Cuisine, stationID, a.Latitude, a.Longitude).Scan(&id)
        if err == nil {
            _, err = tx.Exec(ctx, `INSERT INTO merchants.merchant_branches(merchant_id,name,address,is_active,created_by,latitude,longitude) VALUES($1,$2,$3,false,$4,$5,$6)`, id, a.Name, a.Address, userID, a.Latitude, a.Longitude)
        }
    } else if kind == "delivery" {
        err = tx.QueryRow(ctx, `INSERT INTO delivery.delivery_partners(user_id,name,phone,status,approval_status,station_id) VALUES($1,$2,$3,'OFFLINE','PENDING',$4) RETURNING id`, userID, a.Name, a.Phone, stationID).Scan(&id)
        if err == nil {
            _, err = tx.Exec(ctx, `INSERT INTO delivery.vehicles(partner_id,vehicle_type,registration_number) VALUES($1,$2,$3)`, id, a.VehicleType, a.RegistrationNumber)
        }
    } else {
        return "", errors.New("invalid application kind")
    }
    if err != nil {
        return "", err
    }
    a.StationID = stationID
    data, err := json.Marshal(a)
    if err != nil {
        return "", err
    }
    _, err = tx.Exec(ctx, `INSERT INTO public.partner_applications(user_id,kind,resource_id,data,document_ids) VALUES($1,$2,$3,$4,$5::uuid[])`, userID, kind, id, data, a.DocumentIDs)
    if err != nil {
        return "", err
    }
    _, err = tx.Exec(ctx, `INSERT INTO notifications.email_outbox(recipient,subject,body) VALUES($1,'Yember application submitted','Your application has been submitted for administrator review.')`, email)
    if err != nil {
        return "", err
    }
    if err = tx.Commit(ctx); err != nil {
        return "", err
    }
    return id, nil
}
