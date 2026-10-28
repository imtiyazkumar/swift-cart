package contractschema

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/imtiyazkumar/yember-core/internal/legacy/devseed"
	"github.com/imtiyazkumar/yember-core/internal/legacy/finance"
	"github.com/imtiyazkumar/yember-core/internal/testdb"
)

func TestPrepareKeepsBankCiphertextDecryptableAfterNumericMapping(t *testing.T) {
	p := testdb.New(t)
	ctx := context.Background()
	if err := devseed.Seed(ctx, p, "LocalTestPassword!2026"); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	keys, _ := json.Marshal(map[string]string{"preservation": base64.StdEncoding.EncodeToString(key)})
	t.Setenv("FINANCE_BANK_KEYS", string(keys))
	t.Setenv("FINANCE_BANK_ACTIVE_KEY", "preservation")
	svc := finance.New(p)
	original := finance.BankInput{Holder: "Original account holder", Number: "123456789012", Routing: "SBIN0001234"}
	for _, owner := range []string{devseed.OwnerID, devseed.DeliveryUserID} {
		if err := svc.SaveBank(ctx, owner, original); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := PrepareLegacy(ctx, p, "yember_next_bank_test"); err != nil {
		t.Fatal(err)
	}
	rows, err := p.Query(ctx, `SELECT account_number_encrypted FROM yember_next_bank_test.restaurant_bank_accounts UNION ALL SELECT account_number_encrypted FROM yember_next_bank_test.delivery_agent_bank_details`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var text string
		if err = rows.Scan(&text); err != nil {
			t.Fatal(err)
		}
		var envelope struct {
			Format     string `json:"format"`
			KeyID      string `json:"key_id"`
			Owner      string `json:"owner_uuid"`
			Bank       string `json:"bank_uuid"`
			Ciphertext string `json:"ciphertext"`
		}
		if err = json.Unmarshal([]byte(text), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Format != "legacy-v1" || envelope.KeyID != "preservation" {
			t.Fatal("bank envelope changed")
		}
		raw, err := base64.StdEncoding.DecodeString(envelope.Ciphertext)
		if err != nil {
			t.Fatal(err)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			t.Fatal(err)
		}
		plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("yember/bank/"+envelope.Owner+"/"+envelope.Bank))
		if err != nil {
			t.Fatal("import invalidated bank encryption", err)
		}
		var restored finance.BankInput
		if err = json.Unmarshal(plain, &restored); err != nil || restored != original {
			t.Fatal("decrypted bank details changed", err)
		}
		count++
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("bank account missing", count)
	}
}
