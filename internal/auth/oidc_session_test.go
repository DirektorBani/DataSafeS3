package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

func expireSessionCache(store *OIDCSessionStore, sessionID string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.cache, sessionID)
}

func TestOIDCSessionStorePutSeedsIntrospectCache(t *testing.T) {
	store := NewOIDCSessionStore()
	store.Put("fresh", OIDCSession{AccessToken: "tok", Username: "u1", CreatedAt: time.Now().UTC()})

	calls := 0
	introspect := func(context.Context, string) (bool, error) {
		calls++
		return false, nil
	}

	ok, err := store.Active(context.Background(), "fresh", introspect)
	if err != nil || !ok {
		t.Fatalf("expected fresh session trusted without introspection, got ok=%v err=%v", ok, err)
	}
	if calls != 0 {
		t.Fatalf("expected 0 introspection calls for fresh session, got %d", calls)
	}
}

func TestOIDCSessionStoreActiveUsesCache(t *testing.T) {
	store := NewOIDCSessionStore()
	store.Put("sess1", OIDCSession{
		AccessToken: "tok",
		Username:    "u1",
		CreatedAt:   time.Now().UTC().Add(-2 * introspectCacheTTL),
	})
	expireSessionCache(store, "sess1")

	calls := 0
	introspect := func(context.Context, string) (bool, error) {
		calls++
		return true, nil
	}

	ok, err := store.Active(context.Background(), "sess1", introspect)
	if err != nil || !ok {
		t.Fatalf("expected active session, got ok=%v err=%v", ok, err)
	}
	ok, err = store.Active(context.Background(), "sess1", introspect)
	if err != nil || !ok {
		t.Fatalf("expected cached active session, got ok=%v err=%v", ok, err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 introspection call, got %d", calls)
	}
}

func TestOIDCSessionStoreInactiveRemovesSession(t *testing.T) {
	store := NewOIDCSessionStore()
	store.Put("sess2", OIDCSession{
		AccessToken: "tok",
		CreatedAt:   time.Now().UTC().Add(-2 * introspectCacheTTL),
	})
	expireSessionCache(store, "sess2")

	introspect := func(context.Context, string) (bool, error) {
		return false, nil
	}
	ok, err := store.Active(context.Background(), "sess2", introspect)
	if err != nil || ok {
		t.Fatalf("expected inactive session, got ok=%v err=%v", ok, err)
	}
	if _, found := store.Get("sess2"); found {
		t.Fatal("expected session removed after inactive introspection")
	}
}

func TestIssueWithTTLOIDCClaims(t *testing.T) {
	m := NewJWTManager("secret", 24*time.Hour)
	token, err := m.IssueWithTTL(TokenInfo{
		Username:   "sso",
		UserID:     "1",
		Role:       RoleUser,
		AuthSource: AuthSourceOIDC,
		SessionID:  "abc123",
	}, OIDCSessionJWTTTL)
	if err != nil {
		t.Fatal(err)
	}
	info, err := m.Validate(token)
	if err != nil {
		t.Fatal(err)
	}
	if info.AuthSource != AuthSourceOIDC || info.SessionID != "abc123" {
		t.Fatalf("unexpected claims: %+v", info)
	}
}

func TestBuildEndSessionURL(t *testing.T) {
	url, err := BuildEndSessionURL(
		OIDCIssuers{Public: "http://localhost:8180/realms/datasafe"},
		"client",
		"idtok",
		"http://localhost:8080/login",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if url == "" {
		t.Fatal("expected logout url")
	}
	if !strings.Contains(url, "client_id=client") || !strings.Contains(url, "id_token_hint=idtok") || !strings.Contains(url, "post_logout_redirect_uri=") {
		t.Fatalf("unexpected logout url: %s", url)
	}
}
