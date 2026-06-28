package auth

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// OIDCExchangeStore holds single-use OIDC login exchange codes.
type OIDCExchangeStore struct {
	mu    sync.Mutex
	codes map[string]exchangeEntry
	ttl   time.Duration
}

type exchangeEntry struct {
	token     string
	expiresAt time.Time
}

// NewOIDCExchangeStore creates a store with the given TTL (default 60s if zero).
func NewOIDCExchangeStore(ttl time.Duration) *OIDCExchangeStore {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &OIDCExchangeStore{
		codes: make(map[string]exchangeEntry),
		ttl:   ttl,
	}
}

// Issue creates a one-time exchange code for jwtToken.
func (s *OIDCExchangeStore) Issue(jwtToken string) (string, error) {
	code, err := randomExchangeCode()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	s.codes[code] = exchangeEntry{token: jwtToken, expiresAt: time.Now().Add(s.ttl)}
	return code, nil
}

// Redeem returns the JWT for code and removes it (single-use).
func (s *OIDCExchangeStore) Redeem(code string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.purgeLocked()
	e, ok := s.codes[code]
	if !ok || time.Now().After(e.expiresAt) {
		return "", false
	}
	delete(s.codes, code)
	return e.token, true
}

func (s *OIDCExchangeStore) purgeLocked() {
	now := time.Now()
	for k, e := range s.codes {
		if now.After(e.expiresAt) {
			delete(s.codes, k)
		}
	}
}

func randomExchangeCode() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
