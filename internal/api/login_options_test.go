package api

import (
	"os"
	"testing"
)

func TestLocalLoginEnabled(t *testing.T) {
	t.Setenv("STORAGE_LOCAL_LOGIN_ENABLED", "")
	if !localLoginEnabled() {
		t.Fatal("local login default on")
	}
	t.Setenv("STORAGE_LOCAL_LOGIN_ENABLED", "false")
	if localLoginEnabled() {
		t.Fatal("local login should be off")
	}
	t.Setenv("STORAGE_LOCAL_LOGIN_ENABLED", "true")
	if !localLoginEnabled() {
		t.Fatal("local login should be on")
	}
	_ = os.Unsetenv("STORAGE_LOCAL_LOGIN_ENABLED")
}
