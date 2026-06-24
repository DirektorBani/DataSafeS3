package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const mfaTokenPurpose = "mfa_pending"

var ErrInvalidMFAToken = errors.New("invalid mfa token")

type MFAClaims struct {
	UserID  string `json:"uid"`
	Purpose string `json:"purpose"`
	jwt.RegisteredClaims
}

// IssueMFAToken returns a short-lived token for the second login step.
func (m *JWTManager) IssueMFAToken(userID string) (string, error) {
	now := time.Now().UTC()
	claims := MFAClaims{
		UserID:  userID,
		Purpose: mfaTokenPurpose,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ValidateMFAToken parses an MFA pending token and returns the user ID.
func (m *JWTManager) ValidateMFAToken(tokenStr string) (string, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &MFAClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidMFAToken
		}
		return m.secret, nil
	})
	if err != nil {
		return "", ErrInvalidMFAToken
	}
	claims, ok := token.Claims.(*MFAClaims)
	if !ok || !token.Valid || claims.UserID == "" || claims.Purpose != mfaTokenPurpose {
		return "", ErrInvalidMFAToken
	}
	return claims.UserID, nil
}
