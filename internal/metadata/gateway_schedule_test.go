package metadata

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReplicationRuleScheduleRoundTripBolt(t *testing.T) {
	s, err := OpenBolt(filepath.Join(t.TempDir(), "meta.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	last := time.Date(2026, 10, 12, 8, 30, 0, 0, time.UTC)
	in := ReplicationRule{
		ID:              "rule-sched",
		Name:            "nightly",
		SourceBucket:    "src",
		DestConnection:  "conn",
		DestBucket:      "dst",
		Enabled:         true,
		CreatedAt:       last.Add(-time.Hour),
		SyncSchedule:    "15m",
		LastSyncAt:      &last,
		LastSyncStatus:  "failed",
		LastSyncError:   "connection refused",
		LastSyncObjects: 3,
	}
	if err := s.PutReplicationRule(in); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetReplicationRule(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SyncSchedule != "15m" || got.LastSyncStatus != "failed" || got.LastSyncError != "connection refused" || got.LastSyncObjects != 3 {
		t.Fatalf("schedule fields: %+v", got)
	}
	if got.LastSyncAt == nil || !got.LastSyncAt.Equal(last) {
		t.Fatalf("last_sync_at: %+v", got.LastSyncAt)
	}
	list, err := s.ListReplicationRules()
	if err != nil || len(list) != 1 || list[0].SyncSchedule != "15m" {
		t.Fatalf("list: %+v err=%v", list, err)
	}
}
