package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

type AdminClaims struct {
	Username   string `json:"sub"`
	UserID     string `json:"uid"`
	Role       string `json:"role"`
	AuthSource string `json:"src,omitempty"`
	SessionID  string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

type TokenInfo struct {
	Username   string
	UserID     string
	Role       string
	AuthSource string
	SessionID  string
}

type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	if secret == "" {
		secret = "datasafe-jwt-secret"
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &JWTManager{secret: []byte(secret), ttl: ttl}
}

func (m *JWTManager) Issue(info TokenInfo) (string, error) {
	return m.IssueWithTTL(info, 0)
}

func (m *JWTManager) IssueWithTTL(info TokenInfo, ttl time.Duration) (string, error) {
	now := time.Now().UTC()
	if ttl <= 0 {
		ttl = m.ttl
	}
	sessionID := info.SessionID
	if sessionID == "" {
		sessionID = newSessionID()
	}
	claims := AdminClaims{
		Username:   info.Username,
		UserID:     info.UserID,
		Role:       info.Role,
		AuthSource: info.AuthSource,
		SessionID:  sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   info.Username,
			ID:        sessionID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (m *JWTManager) Validate(tokenStr string) (TokenInfo, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &AdminClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return m.secret, nil
	})
	if err != nil {
		return TokenInfo{}, ErrInvalidToken
	}
	claims, ok := token.Claims.(*AdminClaims)
	if !ok || !token.Valid || claims.Username == "" {
		return TokenInfo{}, ErrInvalidToken
	}
	role := claims.Role
	if role == "" {
		role = RoleAdministrator
	}
	sessionID := claims.SessionID
	if sessionID == "" {
		sessionID = claims.ID
	}
	return TokenInfo{
		Username:   claims.Username,
		UserID:     claims.UserID,
		Role:       role,
		AuthSource: claims.AuthSource,
		SessionID:  sessionID,
	}, nil
}
