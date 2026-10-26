package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"os"
	"strings"
	"time"
)

func verifyTOTP(secret, code string, now time.Time) (int64, bool) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil || len(raw) < 20 || len(code) != 6 {
		return 0, false
	}
	for shift := int64(-1); shift <= 1; shift++ {
		step := now.Unix()/30 + shift
		msg := make([]byte, 8)
		binary.BigEndian.PutUint64(msg, uint64(step))
		mac := hmac.New(sha1.New, raw)
		mac.Write(msg)
		digest := mac.Sum(nil)
		offset := digest[len(digest)-1] & 15
		value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
		expected := fmt.Sprintf("%06d", value%1000000)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}
func requireAdminMFA(c echo.Context, pool *pgxpool.Pool, user *User, code string) error {
	if user.Role != "ADMIN" {
		return nil
	}
	var secrets map[string]string
	_ = json.Unmarshal([]byte(os.Getenv("ADMIN_TOTP_SECRETS")), &secrets)
	secret := secrets[user.ID]
	if secret == "" && os.Getenv("ADMIN_MFA_REQUIRED") != "true" {
		return nil
	}
	step, valid := verifyTOTP(secret, code, time.Now())
	if !valid {
		return echo.NewHTTPError(401, "administrator authenticator code required")
	}
	tag, err := pool.Exec(c.Request().Context(), `INSERT INTO auth.admin_mfa_steps(user_id,last_step) VALUES($1,$2) ON CONFLICT(user_id) DO UPDATE SET last_step=EXCLUDED.last_step WHERE auth.admin_mfa_steps.last_step<EXCLUDED.last_step`, user.ID, step)
	if err != nil {
		return echo.NewHTTPError(503, "administrator authentication unavailable")
	}
	if tag.RowsAffected() != 1 {
		return echo.NewHTTPError(401, "authenticator code already used")
	}
	return nil
}
