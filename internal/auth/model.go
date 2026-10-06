package auth

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email,omitempty"`
	Phone        string    `json:"phone,omitempty"`
	FullName     string    `json:"full_name,omitempty"`
	Gender       string    `json:"gender,omitempty"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}

// NewUser creates a User with hashed password.
func NewUser(email, password, role string) (*User, error) {
	if role == "" {
		role = "customer"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &User{Email: email, PasswordHash: string(hash), Role: role}, nil
}

// VerifyPassword checks a plaintext password against the stored hash.
func (u *User) VerifyPassword(password string) error {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
}

// Insert saves the user into the database.
func (u *User) Insert(ctx context.Context, pool *pgxpool.Pool) error {
	query := `INSERT INTO users (email, password_hash, role, created_at) VALUES (NULLIF($1, ''), $2, $3, now()) RETURNING id, created_at`
	return pool.QueryRow(ctx, query, u.Email, u.PasswordHash, u.Role).Scan(&u.ID, &u.CreatedAt)
}

// FindByEmail retrieves a user by email.
func FindByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*User, error) {
	u := &User{}
	query := `SELECT id, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(full_name, ''), password_hash, role, created_at FROM users WHERE email=$1`
	err := pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.Phone, &u.FullName, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func NormalizeIdentifier(id string) string {
	id = strings.TrimSpace(id)
	if strings.Contains(id, "@") {
		return strings.ToLower(id)
	}
	var cleaned strings.Builder
	for _, ch := range id {
		if (ch >= '0' && ch <= '9') || ch == '+' {
			cleaned.WriteRune(ch)
		}
	}
	s := cleaned.String()
	if strings.HasPrefix(s, "+91") {
		s = strings.TrimPrefix(s, "+91")
	}
	return s
}

// FindOrCreateByIdentifier looks up a user by email or phone, or creates a new customer user.
func FindOrCreateByIdentifier(ctx context.Context, pool *pgxpool.Pool, identifier string) (*User, error) {
	identifier = NormalizeIdentifier(identifier)
	isEmail := strings.Contains(identifier, "@")
	var email, phone string
	if isEmail {
		email = strings.ToLower(identifier)
		phone = ""
	} else {
		phone = identifier
		email = ""
	}

	u := &User{}
	var query string
	var err error

	if isEmail {
		query = `SELECT id, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(full_name, ''), COALESCE(gender, ''), role, created_at FROM users WHERE email=$1 AND deleted_at IS NULL`
		err = pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.Phone, &u.FullName, &u.Gender, &u.Role, &u.CreatedAt)
	} else {
		query = `SELECT id, COALESCE(email, ''), COALESCE(phone, ''), COALESCE(full_name, ''), COALESCE(gender, ''), role, created_at FROM users WHERE phone=$1 AND deleted_at IS NULL`
		err = pool.QueryRow(ctx, query, phone).Scan(&u.ID, &u.Email, &u.Phone, &u.FullName, &u.Gender, &u.Role, &u.CreatedAt)
	}

	if err == nil {
		_, _ = pool.Exec(ctx, `UPDATE users SET last_login_at=now() WHERE id=$1`, u.ID)
		return u, nil
	}

	// Create user
	role := "customer"
	var fullName string
	if isEmail {
		parts := strings.Split(email, "@")
		fullName = parts[0]
	} else {
		fullName = "User " + phone
	}

	insertUserQuery := `INSERT INTO users (email, phone, full_name, password_hash, role, created_at, updated_at, last_login_at)
		VALUES (NULLIF($1, ''), NULLIF($2, ''), $3, '', $4, now(), now(), now())
		RETURNING id, created_at`
	err = pool.QueryRow(ctx, insertUserQuery, email, phone, fullName, role).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	u.Email = email
	u.Phone = phone
	u.FullName = fullName
	u.Role = role

	// Also ensure customer entry in public.customers
	insertCustomerQuery := `INSERT INTO customers (id, name, email, phone, created_at, updated_at)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), now(), now())
		ON CONFLICT (id) DO UPDATE SET phone=EXCLUDED.phone, email=EXCLUDED.email, updated_at=now()`
	_, _ = pool.Exec(ctx, insertCustomerQuery, u.ID, fullName, email, phone)

	return u, nil
}

// CreateOTP stores an OTP record in auth.otp_codes.
func CreateOTP(ctx context.Context, pool *pgxpool.Pool, identifier, code string) error {
	identifier = NormalizeIdentifier(identifier)
	query := `INSERT INTO auth.otp_codes (identifier, otp_code, expires_at, is_used, created_at)
		VALUES ($1, $2, NOW() + INTERVAL '10 minutes', false, NOW())`
	_, err := pool.Exec(ctx, query, identifier, code)
	return err
}

// VerifyOTPInDB checks auth.otp_codes for an active matching code and marks it used.
func VerifyOTPInDB(ctx context.Context, pool *pgxpool.Pool, identifier, code string) (bool, error) {
	identifier = NormalizeIdentifier(identifier)
	code = strings.TrimSpace(code)
	query := `UPDATE auth.otp_codes
		SET is_used = true
		WHERE id = (
			SELECT id FROM auth.otp_codes
			WHERE (identifier = $1 OR identifier = '+' || $1 OR identifier = '+91' || $1) AND otp_code = $2 AND is_used = false AND expires_at > NOW()
			ORDER BY created_at DESC
			LIMIT 1
		)
		RETURNING id`
	var id string
	err := pool.QueryRow(ctx, query, identifier, code).Scan(&id)
	if err != nil {
		return false, nil
	}
	return true, nil
}


