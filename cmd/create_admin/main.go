package main

import (
	"context"
	"log"
	"net/mail"
	"os"
	"strings"

	"github.com/imtiyazkumar/yember-core/config"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

// Operator-only bootstrap. No public API can grant administrator privileges.
func main() {
	_ = godotenv.Load()
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_EMAIL")))
	password := os.Getenv("ADMIN_PASSWORD")
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(password) < 12 || len(password) > 72 {
		log.Fatal("set ADMIN_EMAIL and ADMIN_PASSWORD (12-72 bytes) securely in the process environment")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("password hashing failed")
	}
	pool, err := pgxpool.New(context.Background(), cfg.PostgresDSN)
	if err != nil {
		log.Fatal("database configuration failed")
	}
	defer pool.Close()
	_, err = pool.Exec(context.Background(), `INSERT INTO users(email,password_hash,role,full_name,status) VALUES($1,$2,'ADMIN','Platform administrator','ACTIVE')`, email, string(hash))
	if err != nil {
		log.Fatal("administrator creation failed; an account may already exist. Existing accounts are never promoted or overwritten by this command")
	}
	log.Print("administrator created")
}
