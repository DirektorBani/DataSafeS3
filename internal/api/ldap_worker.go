package api

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

type ldapSyncResult struct {
	Created    int `json:"created"`
	Updated    int `json:"updated"`
	Suspended  int `json:"suspended"`
	TotalFound int `json:"total_found"`
}

func (r ldapSyncResult) synced() int {
	return r.Created + r.Updated
}

// LDAPSyncInterval exposes effective scheduled sync interval for tests and diagnostics.
func (s *Server) LDAPSyncInterval(cfg metadata.SystemConfig) time.Duration {
	return s.ldapSyncInterval(cfg)
}

func (s *Server) ldapSyncInterval(cfg metadata.SystemConfig) time.Duration {
	if !cfg.LDAP.Enabled || cfg.LDAP.URL == "" {
		return 0
	}
	mins := cfg.LDAP.SyncIntervalMinutes
	if v := os.Getenv("STORAGE_LDAP_SYNC_INTERVAL_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n <= 0 {
				return 0
			}
			return time.Duration(n) * time.Minute
		}
	}
	if mins <= 0 {
		return 0
	}
	return time.Duration(mins) * time.Minute
}

func (s *Server) runLDAPSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	var lastRun time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg, err := s.meta.GetSystemConfig()
			if err != nil {
				continue
			}
			cfg = s.mergeEnvConfig(cfg)
			interval := s.ldapSyncInterval(cfg)
			if interval <= 0 {
				continue
			}
			if !lastRun.IsZero() && time.Since(lastRun) < interval {
				continue
			}
			res, err := s.performLDAPSync(cfg)
			lastRun = time.Now().UTC()
			if err != nil {
				observability.IncLDAPSync("error")
				continue
			}
			observability.IncLDAPSync("success")
			s.logLDAPSyncActivity(res)
		}
	}
}

func (s *Server) performLDAPSync(cfg metadata.SystemConfig) (ldapSyncResult, error) {
	users, err := auth.ListLDAPUsers(s.ldapSettingsFrom(cfg), 10000)
	if err != nil {
		return ldapSyncResult{}, err
	}
	ldapByName := make(map[string]auth.LDAPUser, len(users))
	for _, lu := range users {
		ldapByName[lu.Username] = lu
	}

	var res ldapSyncResult
	res.TotalFound = len(users)
	for _, lu := range users {
		role := auth.MapLDAPRole(lu.Groups, cfg.LDAP.GroupRoleMap, metadata.RoleUser)
		existing, err := s.meta.GetUserByUsername(lu.Username)
		if err != nil {
			hash, _ := auth.HashPassword(randomHex(8))
			rec := metadata.UserRecord{
				ID:           randomID(),
				Username:     lu.Username,
				Email:        lu.Email,
				PasswordHash: hash,
				Role:         role,
				Status:       metadata.StatusActive,
				TenantID:     metadata.DefaultTenantID,
				AuthSource:   "ldap",
				CreatedAt:    time.Now().UTC(),
			}
			if err := s.meta.PutUser(rec); err == nil {
				res.Created++
			}
			continue
		}
		if existing.AuthSource != "" && existing.AuthSource != "ldap" {
			continue
		}
		changed := false
		if existing.Email != lu.Email {
			existing.Email = lu.Email
			changed = true
		}
		if existing.Role != role {
			existing.Role = role
			changed = true
		}
		if existing.Status != metadata.StatusActive {
			existing.Status = metadata.StatusActive
			changed = true
		}
		if existing.AuthSource != "ldap" {
			existing.AuthSource = "ldap"
			changed = true
		}
		if changed {
			if err := s.meta.UpdateUser(existing); err == nil {
				res.Updated++
			}
		}
	}

	allUsers, err := s.meta.ListUsers()
	if err != nil {
		return res, nil
	}
	for _, u := range allUsers {
		if u.AuthSource != "ldap" || u.Role == metadata.RoleAdministrator {
			continue
		}
		if _, ok := ldapByName[u.Username]; ok || u.Status != metadata.StatusActive {
			continue
		}
		u.Status = metadata.StatusSuspended
		if err := s.meta.UpdateUser(u); err == nil {
			res.Suspended++
		}
	}
	return res, nil
}

func (s *Server) logLDAPSyncActivity(res ldapSyncResult) {
	msg := "ldap sync: created=" + strconv.Itoa(res.Created) +
		" updated=" + strconv.Itoa(res.Updated) +
		" suspended=" + strconv.Itoa(res.Suspended) +
		" total_found=" + strconv.Itoa(res.TotalFound)
	s.logActivityAs("system", "", metadata.ActionSettingsChanged, "ldap", msg)
}
