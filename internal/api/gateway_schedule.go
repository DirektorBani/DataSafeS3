package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

const (
	scheduledGatewaySyncTick = 30 * time.Second
	minSyncSchedule          = time.Minute
	maxSyncSchedule          = 168 * time.Hour
)

// ruleUsesGlobalFullSync is true when the rule still follows
// STORAGE_GATEWAY_FULL_SYNC_INTERVAL (default 1h). A non-empty schedule,
// including "off", is excluded so it is not scanned twice.
func ruleUsesGlobalFullSync(rule metadata.ReplicationRule) bool {
	return rule.Enabled && strings.TrimSpace(rule.SyncSchedule) == ""
}

func normalizeSyncSchedule(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	switch s {
	case "", "default":
		return "", nil
	case "off":
		return "off", nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < minSyncSchedule || d > maxSyncSchedule {
		return "", fmt.Errorf("sync_schedule must be empty, off, or a duration from 1m to 168h")
	}
	return formatSyncInterval(d), nil
}

func formatSyncInterval(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int(d/time.Minute))
	}
	return d.String()
}

func parseRuleSyncInterval(schedule string) (time.Duration, bool) {
	s := strings.TrimSpace(strings.ToLower(schedule))
	if s == "" || s == "off" || s == "default" {
		return 0, false
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < minSyncSchedule || d > maxSyncSchedule {
		return 0, false
	}
	return d, true
}

// scheduledSyncDue is false for an empty schedule and for "off".
// A duration is due when the rule has never finished a sync, or the interval has elapsed.
func scheduledSyncDue(rule metadata.ReplicationRule, now time.Time) bool {
	if !rule.Enabled {
		return false
	}
	d, ok := parseRuleSyncInterval(rule.SyncSchedule)
	if !ok {
		return false
	}
	if rule.LastSyncAt == nil || rule.LastSyncAt.IsZero() {
		return true
	}
	return !now.Before(rule.LastSyncAt.Add(d))
}

func (s *Server) runScheduledGatewaySync(ctx context.Context) {
	ticker := time.NewTicker(scheduledGatewaySyncTick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runDueScheduledSyncs(ctx)
		}
	}
}

// RunDueScheduledSyncs copies rules whose per-rule interval is due.
// It does not run at process start; the worker waits for the first tick.
func (s *Server) RunDueScheduledSyncs(ctx context.Context) {
	s.runDueScheduledSyncs(ctx)
}

func (s *Server) runDueScheduledSyncs(ctx context.Context) {
	rules, err := s.meta.ListReplicationRules()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, rule := range rules {
		if !scheduledSyncDue(rule, now) {
			continue
		}
		if !s.tryBeginGatewaySync(rule.ID) {
			continue
		}
		s.finishSyncJob(ctx, rule)
		s.endGatewaySync(rule.ID)
	}
}

func (s *Server) tryBeginGatewaySync(id string) bool {
	s.gatewaySyncMu.Lock()
	defer s.gatewaySyncMu.Unlock()
	if s.gatewaySyncing == nil {
		s.gatewaySyncing = map[string]struct{}{}
	}
	if _, ok := s.gatewaySyncing[id]; ok {
		return false
	}
	s.gatewaySyncing[id] = struct{}{}
	return true
}

func (s *Server) endGatewaySync(id string) {
	s.gatewaySyncMu.Lock()
	defer s.gatewaySyncMu.Unlock()
	delete(s.gatewaySyncing, id)
}

func (s *Server) finishSyncJob(ctx context.Context, rule metadata.ReplicationRule) metadata.SyncJob {
	job := metadata.SyncJob{
		ID:        randomID(),
		RuleID:    rule.ID,
		Status:    metadata.SyncJobRunning,
		StartedAt: time.Now().UTC(),
	}
	if err := s.meta.PutSyncJob(job); err != nil {
		job.Status = metadata.SyncJobFailed
		job.Message = err.Error()
		job.Errors = 1
		return job
	}
	synced, errs, msg := s.runSyncJob(ctx, rule)
	now := time.Now().UTC()
	job.Objects = synced
	job.Errors = errs
	job.Message = msg
	job.Status = metadata.SyncJobCompleted
	if errs > 0 {
		job.Status = metadata.SyncJobFailed
	}
	job.EndedAt = &now
	_ = s.meta.PutSyncJob(job)

	fresh, err := s.meta.GetReplicationRule(rule.ID)
	if err != nil {
		return job
	}
	fresh.LastSyncAt = &now
	fresh.LastSyncStatus = job.Status
	fresh.LastSyncObjects = synced
	if errs > 0 {
		fresh.LastSyncError = msg
	} else {
		fresh.LastSyncError = ""
	}
	_ = s.meta.PutReplicationRule(fresh)
	return job
}

func (s *Server) handlePatchReplicationRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rule, err := s.meta.GetReplicationRule(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var body struct {
		SyncSchedule *string `json:"sync_schedule"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SyncSchedule == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "sync_schedule required"})
		return
	}
	sched, err := normalizeSyncSchedule(*body.SyncSchedule)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	rule.SyncSchedule = sched
	if err := s.meta.PutReplicationRule(rule); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rule": rule})
}
