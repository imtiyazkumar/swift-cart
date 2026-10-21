package auth

import (
    "fmt"
    "github.com/golang-jwt/jwt/v5"
    "github.com/google/uuid"
    "time"
)

type Claims struct {
    UserID    string `json:"sub"`
    Role      string `json:"role"`
    TokenType string `json:"token_type"`
    jwt.RegisteredClaims
}

// GenerateAccessToken creates a JWT valid for the configured TTL (1 hour).
func GenerateAccessToken(secret, userID, role string) (string, error) {
    now := time.Now()
    claims := Claims{
        UserID:    userID,
        Role:      role,
        TokenType: "access",
        RegisteredClaims: jwt.RegisteredClaims{
            ID:        uuid.NewString(),
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(15 * time.Minute)),
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(secret))
}

// GenerateRefreshToken creates a long‑lived JWT (7 days).
func GenerateRefreshToken(secret, userID, role string) (string, error) {
    now := time.Now()
    claims := Claims{
        UserID:    userID,
        Role:      role,
        TokenType: "refresh",
        RegisteredClaims: jwt.RegisteredClaims{
            ID:        uuid.NewString(),
            IssuedAt:  jwt.NewNumericDate(now),
            ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(secret))
}

// ParseToken validates a token and returns its claims.
func ParseToken(secret, tokenStr string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
        if t.Method != jwt.SigningMethodHS256 {
            return nil, fmt.Errorf("unexpected signing method")
        }
        return []byte(secret), nil
    })
    if err != nil {
        return nil, err
    }
    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }
    return nil, fmt.Errorf("invalid token")
}
