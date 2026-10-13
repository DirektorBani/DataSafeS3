package auth

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"image/png"
	"io"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const totpIssuer = "Датасейф S3"

// GenerateTOTPEnrollment creates a new TOTP secret and enrollment artifacts (RFC 6238: 6 digits, 30s, SHA1).
func GenerateTOTPEnrollment(account string) (secret, otpauthURL, qrPNGBase64 string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: account,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", "", err
	}
	secret = key.Secret()
	otpauthURL = key.URL()
	qrPNGBase64, err = qrPNGFromKey(key, 200)
	if err != nil {
		return "", "", "", err
	}
	return secret, otpauthURL, qrPNGBase64, nil
}

func qrPNGFromKey(key *otp.Key, size int) (string, error) {
	img, err := key.Image(size, size)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// GenerateTOTPSecret is kept for compatibility; prefer GenerateTOTPEnrollment.
func GenerateTOTPSecret(issuer, account string) (secret string, qrURL string, err error) {
	if issuer == "" {
		issuer = totpIssuer
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: account,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

func ValidateTOTP(secret, code string) bool {
	_, ok := TOTPStep(secret, code, time.Now())
	return ok
}

// TOTPStep returns the 30-second step that matches code, checking the current
// step and one step on either side. The caller must claim the step so the same
// code cannot be reused for login or step-up.
func TOTPStep(secret, code string, at time.Time) (int64, bool) {
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return 0, false
	}
	for _, delta := range []int64{0, -1, 1} {
		t := at.Add(time.Duration(delta) * 30 * time.Second)
		got, err := totp.GenerateCode(secret, t)
		if err != nil || subtle.ConstantTimeCompare([]byte(got), []byte(code)) != 1 {
			continue
		}
		return t.Unix() / 30, true
	}
	return 0, false
}

func GenerateTOTPCode(secret string, at time.Time) (string, error) {
	return totp.GenerateCode(secret, at)
}

func deriveAESKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

const encryptedSecretPrefix = "aes:"

// EncryptTOTPSecret encrypts a TOTP secret for storage (AES-256-GCM).
func EncryptTOTPSecret(jwtSecret, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	block, err := aes.NewCipher(deriveAESKey(jwtSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return encryptedSecretPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptTOTPSecret decrypts a stored TOTP secret. Plaintext values (legacy/pending) pass through.
func DecryptTOTPSecret(jwtSecret, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, encryptedSecretPrefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, encryptedSecretPrefix))
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(deriveAESKey(jwtSecret))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", ErrInvalidToken
	}
	plain, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DecryptTOTPSecretWithFallback tries primary key then fallback (JWT secret) for key rotation.
func DecryptTOTPSecretWithFallback(primary, fallback, stored string) (string, error) {
	plain, err := DecryptTOTPSecret(primary, stored)
	if err == nil {
		return plain, nil
	}
	if fallback != "" && fallback != primary {
		return DecryptTOTPSecret(fallback, stored)
	}
	return "", err
}

func GenerateRecoveryCodes(n int) ([]string, error) {
	out := make([]string, n)
	for i := 0; i < n; i++ {
		b := make([]byte, 5)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		out[i] = strings.ToUpper(hex.EncodeToString(b))
	}
	return out, nil
}

func HashRecoveryCode(code string) (string, error) {
	return HashPassword(strings.ToUpper(strings.TrimSpace(code)))
}

func CheckRecoveryCode(hash, code string) bool {
	return CheckPassword(hash, strings.ToUpper(strings.TrimSpace(code)))
}

// ConsumeRecoveryCode verifies a recovery code and removes it from the hashed list (single-use).
func ConsumeRecoveryCode(hashes []string, code string) ([]string, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for i, h := range hashes {
		if CheckRecoveryCode(h, code) {
			out := append([]string{}, hashes...)
			return append(out[:i], out[i+1:]...), true
		}
	}
	return hashes, false
}
