package auth

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

type fakeAuthRepo struct {
	user        *User
	refreshHash string
	refresh     *RefreshToken
	deletedHash string
}

func (f *fakeAuthRepo) CreateUser(ctx context.Context, email, passwordHash, role string) (string, error) {
	return "user-1", nil
}

func (f *fakeAuthRepo) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	return f.user, nil
}

func (f *fakeAuthRepo) GetUserByID(ctx context.Context, userID string) (*User, error) {
	return f.user, nil
}

func (f *fakeAuthRepo) StoreRefreshToken(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error {
	f.refreshHash = tokenHash
	f.refresh = &RefreshToken{UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}
	return nil
}

func (f *fakeAuthRepo) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	if f.refresh != nil && f.refresh.TokenHash == tokenHash {
		return f.refresh, nil
	}
	return nil, nil
}

func (f *fakeAuthRepo) DeleteRefreshToken(ctx context.Context, tokenHash string) error {
	f.deletedHash = tokenHash
	return nil
}

func (f *fakeAuthRepo) CreateSession(ctx context.Context, sess Session) error {
	return nil
}

func TestVerifyOTPRejectsInvalidCode(t *testing.T) {
	svc := NewService(&fakeAuthRepo{}, &Config{JwtSecret: "secret", JwtExpireMins: 15, RefreshExpire: 30}, zap.NewNop())

	if _, err := svc.VerifyOTP(context.Background(), "user@example.test", "000000"); err == nil {
		t.Fatal("expected invalid OTP error")
	}
}

func TestIssueAndRefreshTokensRotatesRefreshToken(t *testing.T) {
	repo := &fakeAuthRepo{user: &User{ID: "user-1", Email: "user@example.test", Role: "CUSTOMER"}}
	svc := NewService(repo, &Config{JwtSecret: "secret", JwtExpireMins: 15, RefreshExpire: 30}, zap.NewNop())

	access, refresh, err := svc.IssueTokens(context.Background(), repo.user)
	if err != nil {
		t.Fatalf("issue tokens failed: %v", err)
	}
	if access == "" || refresh == "" || repo.refreshHash == "" {
		t.Fatal("expected access token, refresh token, and stored refresh hash")
	}

	newAccess, newRefresh, err := svc.RefreshTokens(context.Background(), refresh)
	if err != nil {
		t.Fatalf("refresh tokens failed: %v", err)
	}
	if newAccess == "" || newRefresh == "" {
		t.Fatal("expected rotated token pair")
	}
	if repo.deletedHash == "" {
		t.Fatal("expected old refresh token to be deleted")
	}
}
