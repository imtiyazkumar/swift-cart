package auth

import (
	"testing"
	"time"
)

func TestTOTPKnownVector(t *testing.T) {
	// RFC 6238 SHA-1 test secret; the implementation presents the six-digit suffix.
	if _, ok := verifyTOTP("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "287082", time.Unix(59, 0)); !ok {
		t.Fatal("known authenticator vector rejected")
	}
	if _, ok := verifyTOTP("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "000000", time.Unix(59, 0)); ok {
		t.Fatal("incorrect authenticator accepted")
	}
}
