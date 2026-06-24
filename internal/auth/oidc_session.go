package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	AuthSourceLocal = ""
	AuthSourceOIDC  = "oidc"

	OIDCSessionJWTTTL  = 15 * time.Minute
	introspectCacheTTL = 30 * time.Second
)

// OIDCSession holds IdP tokens for a console session keyed by JWT session id.
type OIDCSession struct {
	AccessToken string
	IDToken     string
	Username    string
	CreatedAt   time.Time
}

// OIDCSessionStore tracks active OIDC-backed console sessions in memory.
type OIDCSessionStore struct {
	mu       sync.RWMutex
	sessions map[string]OIDCSession
	cache    map[string]introspectCacheEntry
}

type introspectCacheEntry struct {
	active    bool
	expiresAt time.Time
}

// NewOIDCSessionStore creates an empty session store.
func NewOIDCSessionStore() *OIDCSessionStore {
	return &OIDCSessionStore{
		sessions: make(map[string]OIDCSession),
		cache:    make(map[string]introspectCacheEntry),
	}
}

// Put registers or replaces a session.
func (s *OIDCSessionStore) Put(sessionID string, sess OIDCSession) {
	if sessionID == "" {
		return
	}
	if sess.CreatedAt.IsZero() {
		sess.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sessionID] = sess
	// Trust a freshly established IdP session until the first introspection window elapses.
	s.cache[sessionID] = introspectCacheEntry{
		active:    true,
		expiresAt: time.Now().Add(introspectCacheTTL),
	}
}

// Get returns a session if present.
func (s *OIDCSessionStore) Get(sessionID string) (OIDCSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[sessionID]
	return sess, ok
}

// Delete removes a session and cached introspection state.
func (s *OIDCSessionStore) Delete(sessionID string) {
	if sessionID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, sessionID)
	delete(s.cache, sessionID)
}

// Active reports whether the OIDC access token for the session is still valid at the IdP.
func (s *OIDCSessionStore) Active(ctx context.Context, sessionID string, introspect func(context.Context, string) (bool, error)) (bool, error) {
	s.mu.RLock()
	sess, ok := s.sessions[sessionID]
	cacheEntry, cached := s.cache[sessionID]
	s.mu.RUnlock()
	if !ok {
		return false, nil
	}
	if cached && time.Now().Before(cacheEntry.expiresAt) {
		return cacheEntry.active, nil
	}
	if !sess.CreatedAt.IsZero() && time.Since(sess.CreatedAt) < introspectCacheTTL {
		return true, nil
	}
	active, err := introspect(ctx, sess.AccessToken)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	s.cache[sessionID] = introspectCacheEntry{
		active:    active,
		expiresAt: time.Now().Add(introspectCacheTTL),
	}
	if !active {
		delete(s.sessions, sessionID)
	}
	s.mu.Unlock()
	return active, nil
}

// OIDCIntrospect checks whether an access token is active via the IdP introspection endpoint.
func OIDCIntrospect(ctx context.Context, issuers OIDCIssuers, clientID, clientSecret, accessToken string, client *http.Client) (bool, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return false, nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	_, _, introspectURL, err := DiscoverOIDCEndpoints(issuers.Internal, client)
	if err != nil || introspectURL == "" {
		introspectURL = strings.TrimSuffix(issuers.Internal, "/") + "/protocol/openid-connect/token/introspect"
	} else {
		introspectURL = RewriteEndpointHost(introspectURL, issuers.Internal)
	}
	form := url.Values{}
	form.Set("token", accessToken)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, introspectURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, nil
	}
	var out struct {
		Active bool `json:"active"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return false, err
	}
	return out.Active, nil
}

// BuildEndSessionURL constructs a Keycloak/OIDC logout redirect URL.
func BuildEndSessionURL(issuers OIDCIssuers, clientID, idTokenHint, postLogoutRedirect string, client *http.Client) (string, error) {
	issuer := strings.TrimSuffix(strings.TrimSpace(issuers.Public), "/")
	if issuer == "" {
		return "", nil
	}
	endSession := issuer + "/protocol/openid-connect/logout"
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err == nil {
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			var doc struct {
				EndSessionEndpoint string `json:"end_session_endpoint"`
			}
			if json.NewDecoder(resp.Body).Decode(&doc) == nil && doc.EndSessionEndpoint != "" {
				endSession = doc.EndSessionEndpoint
			}
		}
	}
	u, err := url.Parse(endSession)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if clientID != "" {
		q.Set("client_id", clientID)
	}
	if idTokenHint != "" {
		q.Set("id_token_hint", idTokenHint)
	}
	if postLogoutRedirect != "" {
		q.Set("post_logout_redirect_uri", postLogoutRedirect)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
