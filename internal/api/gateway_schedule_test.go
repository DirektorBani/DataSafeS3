package api

import (
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestNormalizeSyncSchedule(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"  default ", "", false},
		{"OFF", "off", false},
		{"15m", "15m", false},
		{"1h", "1h", false},
		{"90m", "90m", false},
		{"24h", "24h", false},
		{"168h", "168h", false},
		{"30s", "", true},
		{"169h", "", true},
		{"nope", "", true},
		{"1d", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeSyncSchedule(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%q: expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.in, got, tc.want)
		}
	}
}

func TestScheduledSyncDue(t *testing.T) {
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	last := now.Add(-20 * time.Minute)
	recent := now.Add(-5 * time.Minute)
	cases := []struct {
		name string
		rule metadata.ReplicationRule
		want bool
	}{
		{"empty never due", metadata.ReplicationRule{Enabled: true, SyncSchedule: ""}, false},
		{"off never due", metadata.ReplicationRule{Enabled: true, SyncSchedule: "off"}, false},
		{"disabled duration", metadata.ReplicationRule{Enabled: false, SyncSchedule: "15m"}, false},
		{"15m first run", metadata.ReplicationRule{Enabled: true, SyncSchedule: "15m"}, true},
		{"15m elapsed", metadata.ReplicationRule{Enabled: true, SyncSchedule: "15m", LastSyncAt: &last}, true},
		{"15m inside interval", metadata.ReplicationRule{Enabled: true, SyncSchedule: "15m", LastSyncAt: &recent}, false},
		{"invalid skipped", metadata.ReplicationRule{Enabled: true, SyncSchedule: "nope"}, false},
	}
	for _, tc := range cases {
		if got := scheduledSyncDue(tc.rule, now); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestRuleUsesGlobalFullSync(t *testing.T) {
	if !ruleUsesGlobalFullSync(metadata.ReplicationRule{Enabled: true}) {
		t.Fatal("empty schedule follows the global full scan")
	}
	if ruleUsesGlobalFullSync(metadata.ReplicationRule{Enabled: true, SyncSchedule: "off"}) {
		t.Fatal("off is excluded from the global full scan")
	}
	if ruleUsesGlobalFullSync(metadata.ReplicationRule{Enabled: true, SyncSchedule: "15m"}) {
		t.Fatal("a per-rule interval is excluded from the global full scan")
	}
	if ruleUsesGlobalFullSync(metadata.ReplicationRule{Enabled: false}) {
		t.Fatal("disabled rules are not scanned")
	}
}
