package auth

import (
	"testing"
	"time"
)

func TestOIDCExchangeStore_singleUse(t *testing.T) {
	store := NewOIDCExchangeStore(time.Minute)
	code, err := store.Issue("jwt-token")
	if err != nil {
		t.Fatal(err)
	}
	tok, ok := store.Redeem(code)
	if !ok || tok != "jwt-token" {
		t.Fatalf("first redeem: ok=%v tok=%q", ok, tok)
	}
	_, ok = store.Redeem(code)
	if ok {
		t.Fatal("second redeem should fail")
	}
}

func TestOIDCExchangeStore_expired(t *testing.T) {
	store := NewOIDCExchangeStore(time.Millisecond)
	code, _ := store.Issue("jwt")
	time.Sleep(5 * time.Millisecond)
	if _, ok := store.Redeem(code); ok {
		t.Fatal("expired code should not redeem")
	}
}
