package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
)

type bankInput struct {
	Holder  string `json:"holder"`
	Number  string `json:"account_number"`
	Routing string `json:"routing_code"`
}

func encryptAccount(owner, id string, b bankInput) (string, error) {
	var keys map[string]string
	keyID := os.Getenv("FINANCE_BANK_ACTIVE_KEY")
	if json.Unmarshal([]byte(os.Getenv("FINANCE_BANK_KEYS")), &keys) != nil {
		return "", fmt.Errorf("bank encryption is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(keys[keyID])
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("bank encryption is not configured")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	plain, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	encrypted := gcm.Seal(nonce, nonce, plain, []byte("yember/bank/"+owner+"/"+id))
	envelope, err := json.Marshal(map[string]string{"format": "legacy-v1", "key_id": keyID, "owner_uuid": owner, "bank_uuid": id, "ciphertext": base64.StdEncoding.EncodeToString(encrypted), "last_four": b.Number[len(b.Number)-4:]})
	return string(envelope), err
}
func (s *Service) registerBanks(api *echo.Group) {
	partner := api.Group("/partner/finance", roles("RESTAURANT_OWNER", "DELIVERY_PARTNER"))
	bankEntity := func(c echo.Context) (string, string, int64, error) {
		if person(c).Role == "RESTAURANT_OWNER" {
			id, err := s.restaurantID(c, false)
			return "restaurant_bank_accounts", "restaurant_id", id, err
		}
		id, err := s.riderID(c)
		return "delivery_agent_bank_details", "rider_id", id, err
	}
	partner.GET("/bank", func(c echo.Context) error {
		table, column, id, err := bankEntity(c)
		if err != nil {
			return err
		}
		return s.one(c, `SELECT COALESCE((SELECT jsonb_build_object('id',uuid,'last_four',account_number_encrypted::jsonb->>'last_four','status',verification_status,'account_holder',account_holder,'bank_name',bank_name,'ifsc',ifsc) FROM `+table+" WHERE "+column+`=$1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1),'null'::jsonb)`, id)
	})
	partner.PUT("/bank", func(c echo.Context) error {
		var input bankInput
		if c.Bind(&input) != nil || !validName(input.Holder, 160) || !regexp.MustCompile(`^[0-9]{8,18}$`).MatchString(input.Number) || !regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`).MatchString(input.Routing) {
			return bad("valid holder, bank account number and IFSC required")
		}
		table, column, entity, err := bankEntity(c)
		if err != nil {
			return err
		}
		uid := uuid.NewString()
		encrypted, err := encryptAccount(person(c).UUID, uid, input)
		if err != nil {
			return echo.NewHTTPError(503, "bank encryption is not configured")
		}
		tx, err := s.begin(c, "UPDATE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		if _, err = tx.Exec(c.Request().Context(), `SELECT id FROM users WHERE id=$1 FOR UPDATE`, person(c).ID); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), "UPDATE "+table+" SET deleted_at=now() WHERE "+column+"=$1 AND deleted_at IS NULL", entity); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), "INSERT INTO "+table+"(uuid,"+column+",account_holder,account_number_encrypted,ifsc,verification_status) VALUES($1,$2,$3,$4,$5,'PENDING')", uid, entity, input.Holder, encrypted, input.Routing); err != nil {
			return err
		}
		return commit(c, tx)
	})
	api.POST("/admin/manage/:resource/:id/verify-bank", func(c echo.Context) error {
		table := resources[c.Param("resource")]
		if table != "restaurant_bank_accounts" && table != "delivery_agent_bank_details" {
			return bad("bank record required")
		}
		if err := publicID(c.Param("id")); err != nil {
			return err
		}
		var input struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		}
		if c.Bind(&input) != nil || (input.Status != "VERIFIED" && input.Status != "REJECTED") || !validName(input.Reason, 2000) {
			return bad("verification result and audit reason required")
		}
		tx, err := s.begin(c, "APPROVE")
		if err != nil {
			return err
		}
		defer tx.Rollback(c.Request().Context())
		ownerQuery := `SELECT r.owner_user_id FROM restaurant_bank_accounts b JOIN restaurants r ON r.id=b.restaurant_id WHERE b.uuid=$1 AND b.deleted_at IS NULL FOR UPDATE OF b`
		if table == "delivery_agent_bank_details" {
			ownerQuery = `SELECT r.user_id FROM delivery_agent_bank_details b JOIN riders r ON r.id=b.rider_id WHERE b.uuid=$1 AND b.deleted_at IS NULL FOR UPDATE OF b`
		}
		var owner int64
		if err = tx.QueryRow(c.Request().Context(), ownerQuery, c.Param("id")).Scan(&owner); err != nil {
			return unavailable(err)
		}
		if owner == person(c).ID {
			return bad("independent bank verification required")
		}
		if _, err = tx.Exec(c.Request().Context(), `SELECT set_config('yember.reason',$1,true)`, input.Reason); err != nil {
			return err
		}
		if _, err = tx.Exec(c.Request().Context(), "UPDATE "+table+" SET verification_status=$2 WHERE uuid=$1 AND deleted_at IS NULL", c.Param("id"), input.Status); err != nil {
			return err
		}
		return commit(c, tx)
	}, roles("ADMIN"))
}
func bankLastFour(envelope string) string {
	var data map[string]string
	if json.Unmarshal([]byte(envelope), &data) != nil {
		return ""
	}
	return strings.TrimSpace(data["last_four"])
}
