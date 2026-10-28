package platform

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

func (s *Service) checkoutResponse(c echo.Context, uid, provider string, amount int64, status string) error {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": person(c).UUID, "type": "checkout", "intent": uid, "exp": time.Now().Add(15 * time.Minute).Unix(), "iat": time.Now().Unix()}).SignedString([]byte(s.JWTSecret))
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(strings.TrimRight(os.Getenv("PUBLIC_API_BASE_URL"), "/"), "/api/v1")
	if base == "" {
		base = c.Scheme() + "://" + c.Request().Host
	}
	return c.JSON(200, map[string]any{"payment_intent_id": uid, "provider_order_id": provider, "key_id": s.Gateway.KeyID, "amount_paise": amount, "currency": "INR", "status": status, "url": base + "/api/v1/payments/checkout/" + token})
}
func (s *Service) checkoutScope(c echo.Context) (string, error) {
	token, err := jwt.Parse(c.Param("token"), func(t *jwt.Token) (any, error) { return []byte(s.JWTSecret), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return "", echo.NewHTTPError(401, "checkout session expired")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || claims["type"] != "checkout" {
		return "", echo.NewHTTPError(401, "checkout session is invalid")
	}
	uid, ok := claims["intent"].(string)
	if !ok || publicID(uid) != nil {
		return "", bad("checkout session is invalid")
	}
	subject, ok := claims["sub"].(string)
	if !ok || publicID(subject) != nil {
		return "", bad("checkout session is invalid")
	}
	u := &identity{}
	if err = s.DB.QueryRow(c.Request().Context(), `SELECT `+identityColumns+` FROM users WHERE uuid=$1 AND status='ACTIVE' AND deleted_at IS NULL`, subject).Scan(&u.ID, &u.UUID, &u.Name, &u.Email, &u.Phone, &u.Role); err != nil {
		return "", echo.NewHTTPError(401, "account is unavailable")
	}
	c.Set("identity", u)
	tx, err := s.DB.Begin(c.Request().Context())
	if err != nil {
		return "", err
	}
	defer tx.Rollback(c.Request().Context())
	if _, err = s.intentAccess(c, tx, uid); err != nil {
		return "", err
	}
	return uid, nil
}
func (s *Service) registerCheckoutPage(api *echo.Group) {
	api.GET("/payments/checkout/:token", func(c echo.Context) error {
		uid, err := s.checkoutScope(c)
		if err != nil {
			return err
		}
		if s.Gateway == nil {
			return echo.NewHTTPError(503, "payments are unavailable")
		}
		var provider, status string
		var amount int64
		if err = s.DB.QueryRow(c.Request().Context(), `SELECT provider_order_id,amount_paise,status FROM payment_intents WHERE uuid=$1`, uid).Scan(&provider, &amount, &status); err != nil {
			return bad("provider order is unavailable")
		}
		if status == "SUCCEEDED" {
			return c.HTML(200, "<!doctype html><title>Yember payment</title><h1>Payment verified</h1><p>You can return to the Yember app.</p>")
		}
		if status != "PENDING" {
			return bad("this payment requires reconciliation")
		}
		options, _ := json.Marshal(map[string]any{"key": s.Gateway.KeyID, "amount": amount, "currency": "INR", "order_id": provider, "name": "Yember", "description": "Secure Yember payment", "theme": map[string]string{"color": "#176b4a"}})
		path, _ := json.Marshal("/api/v1/payments/checkout/" + c.Param("token") + "/verify")
		c.Response().Header().Set("Cache-Control", "no-store")
		c.Response().Header().Set("Referrer-Policy", "no-referrer")
		return c.HTML(http.StatusOK, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Yember secure payment</title><style>body{font:16px system-ui;background:#f5f7f6;color:#243e32;padding:12vh 24px;text-align:center}main{max-width:420px;margin:auto;background:white;padding:40px;border-radius:18px;border:1px solid #dce6e0}button{background:#176b4a;color:white;border:0;padding:14px 25px;border-radius:8px;font-size:16px;cursor:pointer}p{line-height:1.6;color:#63746a}</style></head><body><main><h1>Yember payment</h1><p id="message">Complete your payment securely through Razorpay.</p><button id="pay">Continue to payment</button></main><script src="https://checkout.razorpay.com/v1/checkout.js"></script><script>const options=`+string(options)+`;const endpoint=`+string(path)+`;options.handler=async response=>{const button=document.getElementById('pay');button.disabled=true;try{const result=await fetch(endpoint,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(response)});if(!result.ok)throw new Error('Verification is pending. Return to the app and refresh the payment status.');document.getElementById('message').textContent='Payment verified. You can return to the Yember app.';button.hidden=true;}catch(error){document.getElementById('message').textContent=error.message;}};document.getElementById('pay').onclick=()=>new Razorpay(options).open();</script></body></html>`)
	})
	api.POST("/payments/checkout/:token/verify", func(c echo.Context) error {
		uid, err := s.checkoutScope(c)
		if err != nil {
			return err
		}
		var input struct {
			Payment   string `json:"razorpay_payment_id"`
			Signature string `json:"razorpay_signature"`
		}
		if c.Bind(&input) != nil || s.Gateway == nil {
			return bad("payment signature required")
		}
		var provider string
		if err = s.DB.QueryRow(c.Request().Context(), `SELECT provider_order_id FROM payment_intents WHERE uuid=$1`, uid).Scan(&provider); err != nil {
			return err
		}
		if !signature(s.Gateway.Secret, provider+"|"+input.Payment, input.Signature) {
			return bad("invalid payment signature")
		}
		payment, err := s.Gateway.payment(c.Request().Context(), input.Payment)
		if err != nil {
			return echo.NewHTTPError(503, "provider verification unavailable")
		}
		if payment.OrderID != provider || payment.ID != input.Payment {
			return bad("provider payment does not match this checkout")
		}
		tx, err := s.begin(c, "PAY")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if err = s.capture(c, tx, payment); err != nil {
			return err
		}
		return commit(c, tx)
	})
}
