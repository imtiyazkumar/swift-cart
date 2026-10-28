package platform

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"
	"math/big"
	"net/http"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func fmtID(id int64) string { return strconv.FormatInt(id, 10) }
func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *Service) otpHash(phone, code string) string {
	key := os.Getenv("OTP_HASH_SECRET")
	if key == "" {
		key = s.JWTSecret
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(phone + "/" + code))
	return hex.EncodeToString(mac.Sum(nil))
}
func randomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}
func (s *Service) access(u *identity) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": u.UUID, "role": u.Role, "type": "access", "iat": nowUTC().Unix(), "exp": nowUTC().Add(15 * time.Minute).Unix()}).SignedString([]byte(s.JWTSecret))
}

const identityColumns = `id,uuid,COALESCE(full_name,''),COALESCE(email,''),COALESCE(phone,''),role`

func (s *Service) authenticate(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Request().Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return echo.NewHTTPError(401, "sign in required")
		}
		token, err := jwt.Parse(strings.TrimPrefix(header, "Bearer "), func(t *jwt.Token) (any, error) { return []byte(s.JWTSecret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
		if err != nil || !token.Valid {
			return echo.NewHTTPError(401, "session expired")
		}
		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || claims["type"] != "access" {
			return echo.NewHTTPError(401, "invalid access token")
		}
		sub, err := claims.GetSubject()
		if err != nil || publicID(sub) != nil {
			return echo.NewHTTPError(401, "invalid session")
		}
		u := &identity{}
		if err = s.DB.QueryRow(c.Request().Context(), `SELECT `+identityColumns+` FROM users WHERE uuid=$1 AND status='ACTIVE' AND deleted_at IS NULL`, sub).Scan(&u.ID, &u.UUID, &u.Name, &u.Email, &u.Phone, &u.Role); err != nil || u.Role != claims["role"] {
			return echo.NewHTTPError(401, "session unavailable")
		}
		c.Set("identity", u)
		return next(c)
	}
}
func readIdentity(ctx context.Context, tx pgx.Tx, id int64) (*identity, error) {
	u := &identity{}
	err := tx.QueryRow(ctx, `SELECT `+identityColumns+` FROM users WHERE id=$1 AND status='ACTIVE' AND deleted_at IS NULL`, id).Scan(&u.ID, &u.UUID, &u.Name, &u.Email, &u.Phone, &u.Role)
	return u, err
}
func (s *Service) issue(c echo.Context, tx pgx.Tx, u *identity, admin bool) (string, int64, error) {
	raw, err := randomToken()
	if err != nil {
		return "", 0, err
	}
	var id int64
	err = tx.QueryRow(c.Request().Context(), `INSERT INTO refresh_tokens(user_id,token_hash,expires_at,device_label,user_agent) VALUES($1,$2,now()+interval '30 days',$3,$4) RETURNING id`, u.ID, hashToken(raw), func() string {
		if admin {
			return "admin web"
		}
		return "app"
	}(), c.Request().UserAgent()).Scan(&id)
	return raw, id, err
}
func (s *Service) session(c echo.Context, u *identity, raw string, admin bool) error {
	access, err := s.access(u)
	if err != nil {
		return err
	}
	if admin {
		c.SetCookie(&http.Cookie{Name: "yember_admin_refresh", Value: raw, Path: "/api/v1/auth/admin", HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 60 * 60})
		return c.JSON(200, map[string]any{"access_token": access, "user": u})
	}
	return c.JSON(200, map[string]any{"access_token": access, "refresh_token": raw, "user": u})
}
func (s *Service) login(admin bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		var input struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if c.Bind(&input) != nil || len(input.Password) > 72 {
			return bad("invalid credentials")
		}
		tx, err := s.begin(c, "LOGIN")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		u := &identity{}
		var hash string
		if err = tx.QueryRow(c.Request().Context(), `SELECT `+identityColumns+`,COALESCE(password_hash,'') FROM users WHERE lower(email)=lower($1) AND status='ACTIVE' AND deleted_at IS NULL`, strings.TrimSpace(input.Email)).Scan(&u.ID, &u.UUID, &u.Name, &u.Email, &u.Phone, &u.Role, &hash); err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
			return echo.NewHTTPError(401, "invalid credentials")
		}
		if admin && u.Role != "ADMIN" {
			return echo.NewHTTPError(403, "administrator required")
		}
		raw, _, err := s.issue(c, tx, u, admin)
		if err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return unavailable(err)
		}
		return s.session(c, u, raw, admin)
	}
}
func (s *Service) refresh(admin bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		raw := ""
		if admin {
			cookie, err := c.Cookie("yember_admin_refresh")
			if err != nil {
				return echo.NewHTTPError(401, "sign in required")
			}
			raw = cookie.Value
		} else {
			var input struct {
				Token string `json:"refresh_token"`
			}
			if c.Bind(&input) != nil {
				return bad("invalid refresh request")
			}
			raw = input.Token
		}
		if len(raw) < 20 || len(raw) > 512 {
			return echo.NewHTTPError(401, "invalid refresh token")
		}
		tx, err := s.begin(c, "REFRESH")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var id, user int64
		var expires time.Time
		var revoked *time.Time
		if err = tx.QueryRow(c.Request().Context(), `SELECT id,user_id,expires_at,revoked_at FROM refresh_tokens WHERE token_hash=$1 AND deleted_at IS NULL FOR UPDATE`, hashToken(raw)).Scan(&id, &user, &expires, &revoked); err != nil {
			return echo.NewHTTPError(401, "refresh session unavailable")
		}
		if revoked != nil || nowUTC().After(expires) {
			if _, err = tx.Exec(c.Request().Context(), `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE user_id=$1`, user); err != nil {
				return err
			}
			if err = tx.Commit(c.Request().Context()); err != nil {
				return err
			}
			return echo.NewHTTPError(401, "refresh token expired or reused")
		}
		u, err := readIdentity(c.Request().Context(), tx, user)
		if err != nil || admin && u.Role != "ADMIN" {
			return echo.NewHTTPError(401, "account unavailable")
		}
		next, nextID, err := s.issue(c, tx, u, admin)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE refresh_tokens SET revoked_at=now(),last_used_at=now(),replaced_by_token_id=$2 WHERE id=$1`, id, nextID); err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		return s.session(c, u, next, admin)
	}
}
func (s *Service) registerAuth(api *echo.Group) {
	group := api.Group("/auth", noStore)
	group.POST("/login", s.login(false))
	group.POST("/refresh", s.refresh(false))
	admin := group.Group("/admin")
	admin.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if c.Request().Header.Get("X-Yember-Admin") != "1" {
				return echo.NewHTTPError(403, "admin request header required")
			}
			origin := c.Request().Header.Get("Origin")
			if origin != "" {
				allowed := false
				for _, candidate := range s.AllowedOrigins {
					if candidate == origin {
						allowed = true
					}
				}
				if !allowed {
					return echo.NewHTTPError(403, "origin not allowed")
				}
			}
			return next(c)
		}
	})
	admin.POST("/login", s.login(true))
	admin.POST("/session", s.refresh(true))
	logout := func(admin bool) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := ""
			if admin {
				if cookie, err := c.Cookie("yember_admin_refresh"); err == nil {
					raw = cookie.Value
				}
				c.SetCookie(&http.Cookie{Name: "yember_admin_refresh", Path: "/api/v1/auth/admin", MaxAge: -1, HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteStrictMode})
			} else {
				var input struct {
					Token string `json:"refresh_token"`
				}
				if c.Bind(&input) != nil {
					return bad("invalid logout")
				}
				raw = input.Token
			}
			if raw != "" {
				if _, err := s.DB.Exec(c.Request().Context(), `UPDATE refresh_tokens SET revoked_at=COALESCE(revoked_at,now()) WHERE token_hash=$1`, hashToken(raw)); err != nil {
					return unavailable(err)
				}
			}
			return c.NoContent(204)
		}
	}
	group.POST("/logout", logout(false))
	admin.POST("/logout", logout(true))
	signup := func(partner bool) echo.HandlerFunc {
		return func(c echo.Context) error {
			var input struct {
				Email    string `json:"email"`
				Phone    string `json:"phone"`
				Password string `json:"password"`
				Name     string `json:"full_name"`
				Kind     string `json:"kind"`
			}
			if c.Bind(&input) != nil || len(input.Password) < 10 || len(input.Password) > 72 {
				return bad("password must contain 10 to 72 bytes")
			}
			if _, err := mail.ParseAddress(input.Email); err != nil {
				return bad("valid email required")
			}
			role := "CUSTOMER"
			input.Phone = normalizePhone(input.Phone)
			if partner {
				switch input.Kind {
				case "restaurant":
					role = "RESTAURANT_OWNER"
				case "delivery":
					role = "DELIVERY_PARTNER"
				default:
					return bad("restaurant or delivery kind required")
				}
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			tx, err := s.begin(c, "CREATE")
			if err != nil {
				return err
			}
			defer tx.Rollback(c.Request().Context())
			u := &identity{}
			if err = tx.QueryRow(c.Request().Context(), `INSERT INTO users(email,phone,full_name,password_hash,role,status) VALUES(lower($1),NULLIF($2,''),$3,$4,$5,'ACTIVE') RETURNING `+identityColumns, strings.TrimSpace(input.Email), strings.TrimSpace(input.Phone), strings.TrimSpace(input.Name), string(hash), role).Scan(&u.ID, &u.UUID, &u.Name, &u.Email, &u.Phone, &u.Role); err != nil {
				return bad("email or phone already registered")
			}
			raw, _, err := s.issue(c, tx, u, false)
			if err != nil {
				return err
			}
			if err = tx.Commit(c.Request().Context()); err != nil {
				return err
			}
			return s.session(c, u, raw, false)
		}
	}
	group.POST("/signup", signup(false))
	group.POST("/partner-signup", signup(true))
	phone := regexp.MustCompile(`^\+?[0-9]{8,15}$`)
	group.POST("/send-otp", func(c echo.Context) error {
		var input struct {
			Phone      string `json:"phone"`
			Identifier string `json:"identifier"`
		}
		if c.Bind(&input) != nil {
			return bad("phone required")
		}
		if input.Phone == "" {
			input.Phone = input.Identifier
		}
		input.Phone = normalizePhone(input.Phone)
		if !phone.MatchString(input.Phone) {
			return bad("valid phone number required")
		}
		if s.SendOTP == nil {
			return echo.NewHTTPError(503, "phone verification delivery is not configured")
		}
		var limited bool
		if err := s.DB.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM otp_challenges WHERE phone=$1 AND created_at>now()-interval '1 minute')`, input.Phone).Scan(&limited); err != nil {
			return err
		}
		if limited {
			return echo.NewHTTPError(429, "wait one minute before requesting another code")
		}
		value, err := rand.Int(rand.Reader, big.NewInt(1000000))
		if err != nil {
			return err
		}
		code := fmt.Sprintf("%06d", value.Int64())
		tx, err := s.begin(c, "CREATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1,729402))`, input.Phone); err != nil {
			return err
		}
		if err = tx.QueryRow(c.Request().Context(), `SELECT EXISTS(SELECT 1 FROM otp_challenges WHERE phone=$1 AND created_at>now()-interval '1 minute')`, input.Phone).Scan(&limited); err != nil {
			return err
		}
		if limited {
			return echo.NewHTTPError(429, "wait one minute before requesting another code")
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE otp_challenges SET consumed_at=now() WHERE phone=$1 AND consumed_at IS NULL`, input.Phone); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), `INSERT INTO otp_challenges(phone,purpose,otp_hash,expires_at,attempts) VALUES($1,'LOGIN',$2,now()+interval '10 minutes',0)`, input.Phone, s.otpHash(input.Phone, code)); err != nil {
			return err
		}
		if err = s.SendOTP(c.Request().Context(), input.Phone, code); err != nil {
			return echo.NewHTTPError(503, "verification message could not be delivered")
		}
		return commit(c, tx)
	})
	group.POST("/verify-otp", func(c echo.Context) error {
		var input struct {
			Phone      string `json:"phone"`
			Identifier string `json:"identifier"`
			Code       string `json:"code"`
			OTP        string `json:"otp"`
		}
		if c.Bind(&input) != nil {
			return bad("invalid verification")
		}
		if input.Phone == "" {
			input.Phone = input.Identifier
		}
		if input.Code == "" {
			input.Code = input.OTP
		}
		input.Phone = normalizePhone(input.Phone)
		if !phone.MatchString(input.Phone) || len(input.Code) != 6 {
			return bad("phone and six-digit code required")
		}
		tx, err := s.begin(c, "VERIFY")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		var id int64
		var stored string
		var attempts int
		if err = tx.QueryRow(c.Request().Context(), `SELECT id,otp_hash,attempts FROM otp_challenges WHERE phone=$1 AND consumed_at IS NULL AND expires_at>now() AND deleted_at IS NULL ORDER BY id DESC LIMIT 1 FOR UPDATE`, input.Phone).Scan(&id, &stored, &attempts); err != nil {
			return echo.NewHTTPError(401, "code expired")
		}
		valid := attempts < 5 && subtle.ConstantTimeCompare([]byte(stored), []byte(s.otpHash(input.Phone, input.Code))) == 1
		if _, err = tx.Exec(c.Request().Context(), `UPDATE otp_challenges SET attempts=attempts+1,consumed_at=CASE WHEN $2 OR attempts>=4 THEN now() ELSE NULL END WHERE id=$1`, id, valid); err != nil {
			return err
		}
		if !valid {
			if err = tx.Commit(c.Request().Context()); err != nil {
				return err
			}
			return echo.NewHTTPError(401, "invalid code")
		}
		var user int64
		if err = tx.QueryRow(c.Request().Context(), `INSERT INTO users(phone,full_name,role,status,phone_verified_at) VALUES($1,'','CUSTOMER','ACTIVE',now()) ON CONFLICT(phone) DO UPDATE SET phone_verified_at=now() RETURNING id`, input.Phone).Scan(&user); err != nil {
			return unavailable(err)
		}
		if _, err = tx.Exec(c.Request().Context(), `UPDATE otp_challenges SET user_id=$2 WHERE id=$1`, id, user); err != nil {
			return err
		}
		u, err := readIdentity(c.Request().Context(), tx, user)
		if err != nil || u.Role == "ADMIN" {
			return echo.NewHTTPError(403, "password login required for administrators")
		}
		raw, _, err := s.issue(c, tx, u, false)
		if err != nil {
			return err
		}
		if err = tx.Commit(c.Request().Context()); err != nil {
			return err
		}
		return s.session(c, u, raw, false)
	})
}
