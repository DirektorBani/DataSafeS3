package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// RFC 6238 Appendix B test vector (SHA1, 30s, 6 digits).
const rfc6238TestSecret = "JBSWY3DPEHPK3PXP"

func TestTOTPKnownSecret(t *testing.T) {
	// RFC 6238 / pquerna: secret JBSWY3DPEHPK3PXP, Unix 59s → TOTP counter 1 → 996554 (SHA1, 6 digits, 30s).
	at := time.Unix(59, 0).UTC()
	opts := totp.ValidateOpts{
		Period:    30,
		Skew:      0,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
	code, err := totp.GenerateCodeCustom(rfc6238TestSecret, at, opts)
	if err != nil {
		t.Fatal(err)
	}
	if code != "996554" {
		t.Fatalf("expected 996554, got %s", code)
	}
	valid, err := totp.ValidateCustom(code, rfc6238TestSecret, at, opts)
	if err != nil || !valid {
		t.Fatal("ValidateCustom failed for known vector")
	}
	if ValidateTOTP(rfc6238TestSecret, "000000") {
		t.Fatal("expected invalid code to fail")
	}
}

func TestTOTPEncryptionRoundTrip(t *testing.T) {
	jwtSecret := "test-jwt-secret"
	plain := "JBSWY3DPEHPK3PXP"
	enc, err := EncryptTOTPSecret(jwtSecret, plain)
	if err != nil {
		t.Fatal(err)
	}
	if enc == plain {
		t.Fatal("expected encrypted value")
	}
	got, err := DecryptTOTPSecret(jwtSecret, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("decrypt: want %q got %q", plain, got)
	}
}

func TestTOTPEncryptionKeyFallback(t *testing.T) {
	jwtSecret := "jwt-key"
	mfaKey := "dedicated-mfa-key"
	plain := "JBSWY3DPEHPK3PXP"
	enc, err := EncryptTOTPSecret(jwtSecret, plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptTOTPSecretWithFallback(mfaKey, jwtSecret, enc)
	if err != nil {
		t.Fatal(err)
	}
	if got != plain {
		t.Fatalf("fallback decrypt: %q", got)
	}
}

func TestRecoveryCodeSingleUse(t *testing.T) {
	codes, err := GenerateRecoveryCodes(3)
	if err != nil {
		t.Fatal(err)
	}
	hashes := make([]string, len(codes))
	for i, c := range codes {
		h, err := HashRecoveryCode(c)
		if err != nil {
			t.Fatal(err)
		}
		hashes[i] = h
	}
	remaining, ok := ConsumeRecoveryCode(hashes, codes[1])
	if !ok {
		t.Fatal("expected recovery code to match")
	}
	if len(remaining) != 2 {
		t.Fatalf("expected 2 remaining, got %d", len(remaining))
	}
	_, ok = ConsumeRecoveryCode(remaining, codes[1])
	if ok {
		t.Fatal("recovery code should be single-use")
	}
}

func TestMFATokenIssueValidate(t *testing.T) {
	m := NewJWTManager("test", time.Hour)
	token, err := m.IssueMFAToken("user-123")
	if err != nil {
		t.Fatal(err)
	}
	uid, err := m.ValidateMFAToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if uid != "user-123" {
		t.Fatalf("got %q", uid)
	}
	if _, err := m.ValidateMFAToken("bad"); err == nil {
		t.Fatal("expected invalid token error")
	}
}

func TestGenerateTOTPEnrollment(t *testing.T) {
	secret, url, qr, err := GenerateTOTPEnrollment("admin")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || url == "" || qr == "" {
		t.Fatalf("empty enrollment artifacts: secret=%q url=%q qr=%q", secret, url, qr)
	}
	if !ValidateTOTP(secret, mustCurrentCode(t, secret)) {
		t.Fatal("generated secret should validate current code")
	}
}

func mustCurrentCode(t *testing.T, secret string) string {
	t.Helper()
	code, err := GenerateTOTPCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return code
}
