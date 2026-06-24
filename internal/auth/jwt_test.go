package auth

import (
	"testing"
	"time"
)

func TestJWTIssueValidate(t *testing.T) {
	m := NewJWTManager("test-secret", time.Hour)
	token, err := m.Issue(TokenInfo{Username: "admin", UserID: "1", Role: RoleAdministrator})
	if err != nil {
		t.Fatal(err)
	}
	info, err := m.Validate(token)
	if err != nil {
		t.Fatal(err)
	}
	if info.Username != "admin" || info.Role != RoleAdministrator {
		t.Fatalf("got %+v", info)
	}
}

func TestJWTInvalid(t *testing.T) {
	m := NewJWTManager("test-secret", time.Hour)
	if _, err := m.Validate("not-a-token"); err == nil {
		t.Fatal("expected error")
	}
}
