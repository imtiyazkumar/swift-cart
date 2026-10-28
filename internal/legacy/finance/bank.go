package finance

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"regexp"
)

type BankInput struct {
	Holder  string `json:"holder"`
	Number  string `json:"account_number"`
	Routing string `json:"routing_code"`
}

var accountPattern = regexp.MustCompile(`^[0-9]{8,18}$`)
var routingPattern = regexp.MustCompile(`^[A-Z]{4}0[A-Z0-9]{6}$`)

func (b BankInput) Validate() error {
	if len(b.Holder) < 2 || len(b.Holder) > 120 || !accountPattern.MatchString(b.Number) || !routingPattern.MatchString(b.Routing) {
		return errors.New("valid account holder, account number and routing code required")
	}
	return nil
}
func bankCipher(keyID string) (cipher.AEAD, error) {
	var keys map[string]string
	if json.Unmarshal([]byte(os.Getenv("FINANCE_BANK_KEYS")), &keys) != nil {
		return nil, errors.New("bank encryption is unavailable")
	}
	key, err := base64.StdEncoding.DecodeString(keys[keyID])
	if err != nil || len(key) != 32 {
		return nil, errors.New("bank encryption key is unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func encryptBank(user, id string, b BankInput) ([]byte, string, error) {
	keyID := os.Getenv("FINANCE_BANK_ACTIVE_KEY")
	gcm, err := bankCipher(keyID)
	if err != nil {
		return nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, "", err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return nil, "", err
	}
	return gcm.Seal(nonce, nonce, raw, []byte("yember/bank/"+user+"/"+id)), keyID, nil
}

// Decryption is deliberately internal; no HTTP endpoint returns the cleartext.
func decryptBank(user, id, keyID string, raw []byte) (BankInput, error) {
	var b BankInput
	gcm, err := bankCipher(keyID)
	if err != nil {
		return b, err
	}
	if len(raw) < gcm.NonceSize() {
		return b, errors.New("invalid encrypted account")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("yember/bank/"+user+"/"+id))
	if err != nil {
		return b, errors.New("bank integrity check failed")
	}
	err = json.Unmarshal(plain, &b)
	return b, err
}
