package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/kms/vaulttransit"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

type kmsSettingsBody struct {
	Addr         string `json:"addr"`
	Namespace    string `json:"namespace"`
	TransitMount string `json:"transit_mount"`
	CACert       string `json:"ca_cert"`
	Auth         string `json:"auth"`
	RoleID       string `json:"role_id"`
	SecretID     string `json:"secret_id"`
	Token        string `json:"token"`
}

func kmsErrorText(err error, timeout time.Duration) (code, message string) {
	switch {
	case errors.Is(err, vaulttransit.ErrBadSecret):
		return "bad_secret", "Неверный Secret ID"
	case errors.Is(err, vaulttransit.ErrNoPermission):
		return "no_permission", "Нет прав на transit/encrypt"
	case errors.Is(err, vaulttransit.ErrTimeout):
		if timeout == 0 || timeout == 5*time.Second {
			return "timeout", "Сервер не отвечает за 5 с"
		}
		return "timeout", "Сервер не отвечает за " + timeout.String()
	default:
		return "unavailable", "Vault недоступен"
	}
}

func (s *Server) kmsSettingsView() map[string]any {
	env, locks := vaulttransit.LoadEnv()
	var stored metadata.VaultKMSConfig
	if cfg, err := s.meta.GetSystemConfig(); err == nil {
		stored = cfg.KMS
	}
	eff := vaulttransit.Merge(env, locks, stored)
	p := s.svc.VaultKMS()
	status := "unconfigured"
	code := ""
	if eff.Enabled() {
		status = "checking"
		if p != nil && p.Probed() {
			if p.Available() {
				status = "connected"
			} else {
				status = "error"
				code = p.LastError()
			}
		} else if p == nil {
			status = "error"
			code = "unavailable"
		}
	}
	_, message := kmsErrorText(codeToErr(code), eff.Timeout)
	if status != "error" {
		message = ""
	}
	auth := "approle"
	if strings.EqualFold(stored.Auth, "token") || eff.Token != "" || eff.TokenFile != "" {
		auth = "token"
	}
	mount := eff.Mount
	if mount == "" {
		mount = "transit"
	}
	return map[string]any{
		"status":        status,
		"provider":      eff.Provider,
		"addr":          eff.Addr,
		"namespace":     eff.Namespace,
		"transit_mount": mount,
		"ca_cert":       eff.CACert,
		"auth":          auth,
		"role_id":       eff.RoleID,
		"secret_id_set": strings.TrimSpace(stored.SecretID) != "" || locks.Secret,
		"token_set":     strings.TrimSpace(stored.Token) != "" || locks.Token,
		"env_locked":    locks,
		"allowlist":     kmsAllowlist(),
		"error":         code,
		"message":       message,
		"kms_available": p != nil && p.Available(),
	}
}

func codeToErr(code string) error {
	switch code {
	case "bad_secret":
		return vaulttransit.ErrBadSecret
	case "no_permission":
		return vaulttransit.ErrNoPermission
	case "timeout":
		return vaulttransit.ErrTimeout
	case "unavailable":
		return kms.ErrUnavailable
	default:
		return nil
	}
}

func kmsAllowlist() string {
	allow := strings.TrimSpace(os.Getenv("STORAGE_KMS_KEY_ALLOWLIST"))
	if allow == "" {
		return "datasafe-*"
	}
	return allow
}

func (s *Server) handleGetKMSSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.kmsSettingsView())
}

func (s *Server) handlePutKMSSettings(w http.ResponseWriter, r *http.Request) {
	var req kmsSettingsBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "config"})
		return
	}
	_, locks := vaulttransit.LoadEnv()
	next := cfg.KMS
	if !locks.Addr {
		next.Addr = strings.TrimSpace(req.Addr)
	}
	if !locks.Namespace {
		next.Namespace = strings.TrimSpace(req.Namespace)
	}
	if !locks.Mount {
		next.TransitMount = strings.TrimSpace(req.TransitMount)
	}
	if !locks.CACert {
		next.CACert = req.CACert
	}
	if !locks.RoleID {
		next.RoleID = strings.TrimSpace(req.RoleID)
	}
	auth := strings.ToLower(strings.TrimSpace(req.Auth))
	if auth != "token" {
		auth = "approle"
	}
	if !locks.Secret && !locks.Token {
		next.Auth = auth
	}
	if !locks.Secret && strings.TrimSpace(req.SecretID) != "" {
		next.SecretID = strings.TrimSpace(req.SecretID)
	}
	if !locks.Token && strings.TrimSpace(req.Token) != "" {
		next.Token = strings.TrimSpace(req.Token)
	}
	if strings.TrimSpace(next.Addr) == "" && !locks.Addr {
		next = metadata.VaultKMSConfig{}
	} else if !locks.Provider {
		next.Provider = kms.ProviderVault
	}
	cfg.KMS = next
	if err := s.meta.PutSystemConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "config"})
		return
	}
	s.refreshVaultFromMeta()
	s.logActivity(r, "kms_settings_changed", "kms", "vault")
	writeJSON(w, http.StatusOK, s.kmsSettingsView())
}

func (s *Server) handleTestKMS(w http.ResponseWriter, r *http.Request) {
	var req kmsSettingsBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	env, locks := vaulttransit.LoadEnv()
	var stored metadata.VaultKMSConfig
	if cfg, err := s.meta.GetSystemConfig(); err == nil {
		stored = cfg.KMS
	}
	eff := vaulttransit.Merge(env, locks, stored)
	if !locks.Addr && strings.TrimSpace(req.Addr) != "" {
		eff.Addr = strings.TrimSpace(req.Addr)
	}
	if !locks.Namespace && req.Namespace != "" {
		eff.Namespace = strings.TrimSpace(req.Namespace)
	}
	if !locks.Mount && strings.TrimSpace(req.TransitMount) != "" {
		eff.Mount = strings.TrimSpace(req.TransitMount)
	}
	if !locks.CACert && req.CACert != "" {
		eff.CACert = req.CACert
	}
	if !locks.RoleID && strings.TrimSpace(req.RoleID) != "" {
		eff.RoleID = strings.TrimSpace(req.RoleID)
	}
	auth := strings.ToLower(strings.TrimSpace(req.Auth))
	if auth == "token" && !locks.Token {
		if strings.TrimSpace(req.Token) != "" {
			eff.Token = strings.TrimSpace(req.Token)
			eff.SecretID = ""
			eff.SecretIDFile = ""
		}
	} else if !locks.Secret && strings.TrimSpace(req.SecretID) != "" {
		eff.SecretID = strings.TrimSpace(req.SecretID)
		eff.Token = ""
		eff.TokenFile = ""
	}
	if !locks.Provider {
		eff.Provider = kms.ProviderVault
	}
	eff.Addr = strings.TrimSpace(eff.Addr)
	if eff.Addr != "" && !strings.Contains(eff.Addr, "://") {
		eff.Addr = "http://" + eff.Addr
	}
	if !eff.Enabled() {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": "not_configured", "message": "Сначала укажите адрес Vault",
		})
		return
	}
	p, err := vaulttransit.Open(eff)
	if err != nil || p == nil {
		code, msg := kmsErrorText(err, eff.Timeout)
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": code, "message": msg})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), p.Timeout())
	defer cancel()
	if err := p.Test(ctx); err != nil {
		code, msg := kmsErrorText(err, p.Timeout())
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": code, "message": msg})
		return
	}
	if live := s.svc.VaultKMS(); live != nil {
		live.Mark(true, "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "Подключено, Transit работает"})
}

func (s *Server) handleGetKMSNodes(w http.ResponseWriter, r *http.Request) {
	beats, err := s.meta.ListNodeHeartbeats(time.Now().Add(-10 * time.Minute))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "nodes"})
		return
	}
	nodes := make([]map[string]any, 0, len(beats))
	for _, b := range beats {
		var available any
		if b.KMSAvailable != nil {
			available = *b.KMSAvailable
		}
		var checked any
		if !b.KMSCheckedAt.IsZero() {
			checked = b.KMSCheckedAt.UTC().Format(time.RFC3339)
		}
		nodes = append(nodes, map[string]any{
			"node_id":    b.NodeID,
			"version":    b.Version,
			"provider":   b.KMSProvider,
			"available":  available,
			"error":      b.KMSError,
			"checked_at": checked,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

func writeObjectStoreErr(w http.ResponseWriter, err error) {
	if errors.Is(err, s3.ErrKMSUnavailable) {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":   "KMS unavailable",
			"message": s3.MsgKMSDown,
		})
		return
	}
	if err == metadata.ErrQuotaExceeded {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "quota exceeded"})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
}
