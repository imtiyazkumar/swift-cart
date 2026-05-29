package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

type Service interface {
	// OTP flow (stubbed for now)
	SendOTP(ctx context.Context, identifier string) error
	VerifyOTP(ctx context.Context, identifier, code string) (*User, error)

	// Token flow
	IssueTokens(ctx context.Context, user *User) (accessToken string, refreshToken string, err error)
	RefreshTokens(ctx context.Context, refreshToken string) (newAccess string, newRefresh string, err error)
}

type authService struct {
	repo   Repository
	cfg    *Config
	log    *zap.Logger
	jwtKey []byte
}

// Config needed for JWT parameters.
type Config struct {
	JwtSecret     string
	JwtExpireMins int
	RefreshExpire int // days
}

func NewService(repo Repository, cfg *Config, log *zap.Logger) Service {
	return &authService{repo: repo, cfg: cfg, log: log, jwtKey: []byte(cfg.JwtSecret)}
}

// SendOTP is a stub – in production it would call SMS/Email provider.
func (s *authService) SendOTP(ctx context.Context, identifier string) error {
	// stub: log and pretend OTP sent
	s.log.Info("OTP sent (stub)", zap.String("identifier", identifier))
	return nil
}

func (s *authService) VerifyOTP(ctx context.Context, identifier, code string) (*User, error) {
	// stub: accept any code "123456"
	if code != "123456" {
		return nil, errors.New("invalid OTP")
	}
	// lookup user by email or phone
	var user *User
	var err error
	if isEmail(identifier) {
		user, err = s.repo.GetUserByEmail(ctx, identifier)
	} else {
		user, err = s.repo.GetUserByEmail(ctx, identifier)
	}
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}
	return user, nil
}

func (s *authService) IssueTokens(ctx context.Context, user *User) (string, string, error) {
	// Access token
	atExpires := time.Now().Add(time.Duration(s.cfg.JwtExpireMins) * time.Minute)
	accessClaims := jwt.MapClaims{
		"sub":  user.ID,
		"role": user.Role,
		"exp":  atExpires.Unix(),
		"iat":  time.Now().Unix(),
	}
	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.jwtKey)
	if err != nil {
		return "", "", err
	}

	// Refresh token (random UUID hashed)
	rawRefresh, err := generateRandomToken()
	if err != nil {
		return "", "", err
	}
	hash := hashToken(rawRefresh)
	exp := time.Now().Add(time.Duration(s.cfg.RefreshExpire) * 24 * time.Hour)
	if err = s.repo.StoreRefreshToken(ctx, user.ID, hash, exp); err != nil {
		return "", "", err
	}
	return accessToken, rawRefresh, nil
}

func (s *authService) RefreshTokens(ctx context.Context, refreshToken string) (string, string, error) {
	hash := hashToken(refreshToken)
	stored, err := s.repo.GetRefreshToken(ctx, hash)
	if err != nil {
		return "", "", err
	}
	if stored == nil || stored.ExpiresAt.Before(time.Now()) {
		return "", "", errors.New("invalid or expired refresh token")
	}
	// fetch user
	user, err := s.repo.GetUserByID(ctx, stored.UserID)
	if err != nil || user == nil {
		return "", "", errors.New("user not found for refresh")
	}
	// rotate: delete old token
	if err = s.repo.DeleteRefreshToken(ctx, hash); err != nil {
		return "", "", err
	}
	// issue new pair
	return s.IssueTokens(ctx, user)
}

// Helper utilities
func isEmail(s string) bool {
	// simple heuristic
	return strings.Contains(s, "@")
}

func generateRandomToken() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
