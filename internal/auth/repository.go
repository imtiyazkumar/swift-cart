package auth

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

// Repository abstracts DB operations for auth.
type Repository interface {
    CreateUser(ctx context.Context, email, passwordHash, role string) (string, error)
    GetUserByEmail(ctx context.Context, email string) (*User, error)
    GetUserByID(ctx context.Context, userID string) (*User, error)
    StoreRefreshToken(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error
    GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
    DeleteRefreshToken(ctx context.Context, tokenHash string) error
    CreateSession(ctx context.Context, sess Session) error
}

type pgRepo struct {
    db  *pgxpool.Pool
    log *zap.Logger
}

func NewPostgresRepo(db *pgxpool.Pool, log *zap.Logger) Repository {
    return &pgRepo{db: db, log: log}
}

// RefreshToken represents a row in auth.refresh_tokens.
type RefreshToken struct {
    ID        string
    UserID    string
    TokenHash string
    ExpiresAt time.Time
    CreatedAt time.Time
}

// Session represents a device/session record.
type Session struct {
    ID         string
    UserID     string
    DeviceInfo string
    IPAddress  string
    UserAgent  string
    LastSeen   time.Time
    CreatedAt  time.Time
}

func (r *pgRepo) CreateUser(ctx context.Context, email, passwordHash, role string) (string, error) {
    var id string
    query := `INSERT INTO users (email, password_hash, role) VALUES ($1,$2,$3) RETURNING id`
    err := r.db.QueryRow(ctx, query, email, passwordHash, role).Scan(&id)
    return id, err
}

func (r *pgRepo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
    var u User
    query := `SELECT id, email, password_hash, role, created_at FROM users WHERE email=$1`
    row := r.db.QueryRow(ctx, query, email)
    err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
    if err == pgx.ErrNoRows {
        return nil, nil
    }
    return &u, err
}

func (r *pgRepo) GetUserByID(ctx context.Context, userID string) (*User, error) {
    var u User
    query := `SELECT id, email, password_hash, role, created_at FROM users WHERE id=$1`
    row := r.db.QueryRow(ctx, query, userID)
    err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
    if err == pgx.ErrNoRows {
        return nil, nil
    }
    return &u, err
}

func (r *pgRepo) StoreRefreshToken(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error {
    query := `INSERT INTO auth.refresh_tokens (user_id, token_hash, expires_at) VALUES ($1,$2,$3)`
    _, err := r.db.Exec(ctx, query, userID, tokenHash, expiresAt)
    return err
}

func (r *pgRepo) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
    var rt RefreshToken
    query := `SELECT id, user_id, token_hash, expires_at, created_at FROM auth.refresh_tokens WHERE token_hash=$1`
    row := r.db.QueryRow(ctx, query, tokenHash)
    err := row.Scan(&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.CreatedAt)
    if err == pgx.ErrNoRows {
        return nil, nil
    }
    return &rt, err
}

func (r *pgRepo) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
    query := `DELETE FROM auth.refresh_tokens WHERE token_hash=$1`
    _, err := r.db.Exec(ctx, query, tokenHash)
    return err
}

func (r *pgRepo) CreateSession(ctx context.Context, sess Session) error {
    query := `INSERT INTO auth.sessions (user_id, device_info, ip_address, user_agent, last_seen) VALUES ($1,$2,$3,$4,$5)`
    _, err := r.db.Exec(ctx, query, sess.UserID, sess.DeviceInfo, sess.IPAddress, sess.UserAgent, sess.LastSeen)
    return err
}
