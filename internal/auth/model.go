package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
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
	query := `INSERT INTO users (email, password_hash, role, created_at) VALUES ($1, $2, $3, now()) RETURNING id, created_at`
	return pool.QueryRow(ctx, query, u.Email, u.PasswordHash, u.Role).Scan(&u.ID, &u.CreatedAt)
}

// FindByEmail retrieves a user by email.
func FindByEmail(ctx context.Context, pool *pgxpool.Pool, email string) (*User, error) {
	u := &User{}
	query := `SELECT id, email, password_hash, role, created_at FROM users WHERE email=$1`
	err := pool.QueryRow(ctx, query, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}
