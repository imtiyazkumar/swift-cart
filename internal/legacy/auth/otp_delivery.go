package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"math/big"
	"net/mail"
	"os"
	"regexp"
	"strings"
)

func otpHash(code string) string {
	key := os.Getenv("OTP_HASH_SECRET")
	if key == "" {
		key = os.Getenv("JWT_SECRET")
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(code))
	return hex.EncodeToString(mac.Sum(nil))
}
func EmailOTP(pool *pgxpool.Pool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var in struct {
			Identifier string `json:"identifier"`
		}
		if c.Bind(&in) != nil {
			return echo.NewHTTPError(400, "valid email or Indian phone number required")
		}
		identifier := NormalizeIdentifier(in.Identifier)
		phone := !strings.Contains(identifier, "@")
		var err error
		if phone {
			if !regexp.MustCompile(`^[6-9][0-9]{9}$`).MatchString(identifier) {
				return echo.NewHTTPError(400, "valid Indian mobile number required")
			}
			if os.Getenv("TEXTBEE_DELIVERY_ENABLED") != "true" || os.Getenv("TEXTBEE_API_KEY") == "" {
				return echo.NewHTTPError(503, "phone verification is not configured")
			}
		} else {
			address, validationErr := mail.ParseAddress(identifier)
			if validationErr != nil || address.Address != identifier {
				return echo.NewHTTPError(400, "valid email required")
			}
			if os.Getenv("EMAIL_DELIVERY_ENABLED") != "true" || os.Getenv("RESEND_API_KEY") == "" || os.Getenv("EMAIL_FROM") == "" {
				return echo.NewHTTPError(503, "verification email unavailable")
			}
		}
		value, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return echo.NewHTTPError(503, "verification unavailable")
		}
		code := fmt.Sprintf("%06d", value.Int64())
		tx, err := pool.Begin(c.Request().Context())
		if err != nil {
			return echo.NewHTTPError(503, "verification unavailable")
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, identifier); err != nil {
			return err
		}
		var limited bool
		if err = tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM public.otp_challenges WHERE identifier=$1 AND created_at>now()-interval '1 minute')`, identifier).Scan(&limited); err != nil {
			return err
		}
		if limited {
			return echo.NewHTTPError(429, "wait before requesting another code")
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE public.otp_challenges SET is_used=true WHERE identifier=$1`, identifier); err != nil {
			return err
		}
		var challenge string
		if err = tx.QueryRow(c.Request().Context(), `INSERT INTO public.otp_challenges(identifier,otp_hash,expires_at,is_used) VALUES($1,$2,now()+interval '10 minutes',false) RETURNING id`, identifier, otpHash(code)).Scan(&challenge); err != nil {
			return err
		}
		message := "Your Yember code is " + code + ". Valid for 10 minutes. Never share sign-in codes."
		if phone {
			_, err = tx.Exec(c.Request().Context(), `INSERT INTO notifications.sms_outbox(challenge_id,recipient,body,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes')`, challenge, "+91"+identifier, message)
		} else {
			_, err = tx.Exec(c.Request().Context(), `INSERT INTO notifications.email_outbox(recipient,subject,body) VALUES($1,'Your Yember verification code',$2)`, identifier, message)
		}
		if err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		return c.JSON(202, map[string]string{"message": "Verification code queued", "identifier": strings.TrimSpace(identifier)})
	}
}
