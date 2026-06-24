package api_test

import (
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestLDAPSyncIntervalDisabledWhenLDAPOff(t *testing.T) {
	srv := testServer(t)
	cfg := metadata.SystemConfig{
		LDAP: metadata.LDAPConfig{Enabled: false, SyncIntervalMinutes: 60},
	}
	if got := srv.LDAPSyncInterval(cfg); got != 0 {
		t.Fatalf("expected 0 when LDAP disabled, got %v", got)
	}
}

func TestLDAPSyncIntervalRespectsZeroDisable(t *testing.T) {
	t.Setenv("STORAGE_LDAP_SYNC_INTERVAL_MINUTES", "0")
	srv := testServer(t)
	cfg := metadata.SystemConfig{
		LDAP: metadata.LDAPConfig{Enabled: true, URL: "ldap://localhost:389", SyncIntervalMinutes: 0},
	}
	if got := srv.LDAPSyncInterval(cfg); got != 0 {
		t.Fatalf("expected 0 when interval env disables sync, got %v", got)
	}
}

func TestLDAPSyncIntervalUsesConfiguredMinutes(t *testing.T) {
	t.Setenv("STORAGE_LDAP_SYNC_INTERVAL_MINUTES", "")
	srv := testServer(t)
	cfg := metadata.SystemConfig{
		LDAP: metadata.LDAPConfig{Enabled: true, URL: "ldap://localhost:389", SyncIntervalMinutes: 15},
	}
	if got := srv.LDAPSyncInterval(cfg); got != 15*time.Minute {
		t.Fatalf("expected 15m, got %v", got)
	}
}

func TestLDAPSyncIntervalZeroDisablesScheduledSync(t *testing.T) {
	t.Setenv("STORAGE_LDAP_SYNC_INTERVAL_MINUTES", "")
	srv := testServer(t)
	cfg := metadata.SystemConfig{
		LDAP: metadata.LDAPConfig{Enabled: true, URL: "ldap://localhost:389"},
	}
	if got := srv.LDAPSyncInterval(cfg); got != 0 {
		t.Fatalf("expected 0 when interval unset/disabled, got %v", got)
	}
}
