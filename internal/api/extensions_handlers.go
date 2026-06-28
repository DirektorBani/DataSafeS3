package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/extensions"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

func (s *Server) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutSystemConfig(w http.ResponseWriter, r *http.Request) {
	var cfg metadata.SystemConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if cfg.TrashRetentionDays == 0 {
		cfg.TrashRetentionDays = 30
	}
	if cfg.TrashRetentionDays < 1 || cfg.TrashRetentionDays > 3650 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "trash_retention_days must be between 1 and 3650"})
		return
	}
	if err := validateLoggingConfig(cfg.Logging); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if cfg.LDAP.Enabled || cfg.LDAP.URL != "" {
		if err := validateLDAPTLS(cfg.LDAP.URL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
	}
	if existing, err := s.meta.GetSystemConfig(); err == nil {
		cfg.InitialSetupCompleted = existing.InitialSetupCompleted
		cfg.AdminFirstLoginCompleted = existing.AdminFirstLoginCompleted
		cfg.AdminPasswordChanged = existing.AdminPasswordChanged
		if cfg.ExternalS3.Endpoint == "" && cfg.ExternalS3.Bucket == "" {
			cfg.ExternalS3 = existing.ExternalS3
		}
	}
	if err := s.meta.PutSystemConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	observability.GlobalSinkManager().Reconfigure(cfg.Logging)
	s.logActivity(r, metadata.ActionSettingsChanged, "system", "config")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListTrash(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	bucket := r.URL.Query().Get("bucket")
	items, err := s.meta.ListTrash(bucket)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !auth.CanSeeAllBuckets(info.Role) {
		var filtered []metadata.TrashRecord
		for _, tr := range items {
			if s.canAccessBucket(info, tr.OriginalBucket) {
				filtered = append(filtered, tr)
			}
		}
		items = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleRestoreTrash(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, _ := authFrom(r)
	tr, err := s.meta.GetTrash(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !s.canAccessBucket(info, tr.OriginalBucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.svc.RestoreFromTrash(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionTrashRestored, "object", tr.OriginalBucket+"/"+tr.OriginalKey)
	s.emitEvent(metadata.EventObjectCreated, map[string]any{
		"bucket": tr.OriginalBucket, "key": tr.OriginalKey,
	})
	writeJSON(w, http.StatusOK, map[string]any{"object": rec})
}

func (s *Server) handlePurgeTrash(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, _ := authFrom(r)
	tr, err := s.meta.GetTrash(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !s.canAccessBucket(info, tr.OriginalBucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if err := s.svc.PurgeTrashItem(r.Context(), id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionTrashPurged, "object", tr.OriginalBucket+"/"+tr.OriginalKey)
	w.WriteHeader(http.StatusNoContent)
}

func hashConsoleToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newConsoleTokenValue() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return "ds_" + hex.EncodeToString(b)
}

func (s *Server) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	userID := ""
	if !auth.IsAdmin(info.Role) {
		userID = info.UserID
	}
	tokens, err := s.meta.ListConsoleTokens(userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type safe struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Username  string    `json:"username"`
		Scopes    []string  `json:"scopes"`
		ExpiresAt time.Time `json:"expires_at"`
		CreatedAt time.Time `json:"created_at"`
	}
	var out []safe
	for _, t := range tokens {
		out = append(out, safe{
			ID: t.ID, Name: t.Name, Username: t.Username,
			Scopes: t.Scopes, ExpiresAt: t.ExpiresAt, CreatedAt: t.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		Name          string   `json:"name"`
		ExpiresDays   int      `json:"expires_days"`
		Scopes        []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	if req.ExpiresDays <= 0 {
		req.ExpiresDays = 90
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"read", "write"}
	}
	raw := newConsoleTokenValue()
	rec := metadata.ConsoleTokenRecord{
		ID:        randomID(),
		Name:      req.Name,
		TokenHash: hashConsoleToken(raw),
		UserID:    info.UserID,
		Username:  info.Username,
		Scopes:    req.Scopes,
		ExpiresAt: time.Now().UTC().Add(time.Duration(req.ExpiresDays) * 24 * time.Hour),
		CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutConsoleToken(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionAccessKeyCreated, "api_token", req.Name)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": rec.ID, "name": rec.Name, "token": raw,
		"scopes": rec.Scopes, "expires_at": rec.ExpiresAt,
	})
}

func (s *Server) handleDeleteAPIToken(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	id := r.PathValue("id")
	rec, err := s.meta.GetConsoleToken(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !auth.IsAdmin(info.Role) && rec.UserID != info.UserID {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if err := s.meta.DeleteConsoleToken(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	s.logActivity(r, metadata.ActionAccessKeyDeleted, "api_token", rec.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	hooks, err := s.meta.ListWebhooks()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hooks})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string            `json:"name"`
		URL     string            `json:"url"`
		Events  []string          `json:"events"`
		Headers map[string]string `json:"headers"`
		Enabled bool              `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "url required"})
		return
	}
	if err := validateOutboundURL(req.URL); err != nil {
		writeOutboundURLError(w, err)
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{metadata.EventObjectCreated, metadata.EventObjectDeleted}
	}
	rec := metadata.WebhookRecord{
		ID: randomID(), Name: req.Name, URL: req.URL,
		Events: req.Events, Headers: req.Headers, Enabled: req.Enabled,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutWebhook(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "webhook", rec.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"webhook": rec})
}

func (s *Server) handleUpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetWebhook(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Name    *string           `json:"name"`
		URL     *string           `json:"url"`
		Events  []string          `json:"events"`
		Headers map[string]string `json:"headers"`
		Enabled *bool             `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name != nil {
		rec.Name = *req.Name
	}
	if req.URL != nil {
		if err := validateOutboundURL(*req.URL); err != nil {
			writeOutboundURLError(w, err)
			return
		}
		rec.URL = *req.URL
	}
	if req.Events != nil {
		rec.Events = req.Events
	}
	if req.Headers != nil {
		rec.Headers = req.Headers
	}
	if req.Enabled != nil {
		rec.Enabled = *req.Enabled
	}
	if err := s.meta.PutWebhook(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhook": rec})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteWebhook(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) validateConsoleToken(token string) (auth.TokenInfo, error) {
	rec, err := s.meta.FindConsoleTokenByHash(hashConsoleToken(token))
	if err != nil {
		return auth.TokenInfo{}, auth.ErrInvalidToken
	}
	if time.Now().UTC().After(rec.ExpiresAt) {
		return auth.TokenInfo{}, auth.ErrInvalidToken
	}
	user, err := s.meta.GetUser(rec.UserID)
	if err != nil {
		return auth.TokenInfo{}, auth.ErrInvalidToken
	}
	if user.Status != metadata.StatusActive {
		return auth.TokenInfo{}, auth.ErrInvalidToken
	}
	return auth.TokenInfo{
		Username: user.Username,
		UserID:   user.ID,
		Role:     user.Role,
	}, nil
}

func (s *Server) maybeSoftDelete(r *http.Request, bucket, key, versionID string) (bool, metadata.TrashRecord, error) {
	cfg, _ := s.meta.GetSystemConfig()
	if !cfg.SoftDeleteEnabled {
		return false, metadata.TrashRecord{}, nil
	}
	if r.URL.Query().Get("permanent") == "true" {
		return false, metadata.TrashRecord{}, nil
	}
	info, _ := authFrom(r)
	tr, err := s.svc.MoveToTrash(r.Context(), bucket, key, versionID, info.Username)
	if err != nil {
		return false, metadata.TrashRecord{}, err
	}
	return true, tr, nil
}

func webhookTemplates() []map[string]string {
	return []map[string]string{
		{"name": "Slack", "url": "https://hooks.slack.com/services/XXX/YYY/ZZZ"},
		{"name": "Discord", "url": "https://discord.com/api/webhooks/ID/TOKEN"},
	}
}

func validateLoggingConfig(cfg metadata.LoggingConfig) error {
	check := func(name string, ep metadata.LogSinkEndpoint) error {
		if !ep.Enabled {
			return nil
		}
		if strings.TrimSpace(ep.Address) == "" {
			return fmt.Errorf("%s: address required when enabled", name)
		}
		if name == "elasticsearch" && ep.Username != "" && ep.Password == "" && ep.Token == "" {
			return fmt.Errorf("elasticsearch: password or API key required when username is set")
		}
		if err := validateOutboundURL(ep.Address); err != nil {
			return err
		}
		return nil
	}
	if err := check("syslog", cfg.Syslog); err != nil {
		return err
	}
	if err := check("loki", cfg.Loki); err != nil {
		return err
	}
	if err := check("elasticsearch", cfg.Elasticsearch); err != nil {
		return err
	}
	if err := check("webhook", cfg.Webhook); err != nil {
		return err
	}
	return nil
}

func (s *Server) handleWebhookTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": webhookTemplates()})
}

func (s *Server) handleHooksTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string `json:"url"`
		Event  string `json:"event"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "url required"})
		return
	}
	if err := validateOutboundURL(req.URL); err != nil {
		writeOutboundURLError(w, err)
		return
	}
	event := strings.TrimSpace(req.Event)
	if event == "" {
		event = "extension.test"
	}
	payload := map[string]any{
		"event":      event,
		"version":    "1",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
		"test":       true,
		"extensions": extensions.EventTypes(),
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, req.URL, strings.NewReader(string(body)))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-DataSafe-Event", event)
	if req.Secret != "" {
		httpReq.Header.Set("X-DataSafe-Signature", signHookPayload(body, req.Secret))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "delivered": false})
		return
	}
	defer resp.Body.Close()
	writeJSON(w, http.StatusOK, map[string]any{
		"delivered":   resp.StatusCode >= 200 && resp.StatusCode < 300,
		"status_code": resp.StatusCode,
		"event":       event,
	})
}

func signHookPayload(body []byte, secret string) string {
	mac := sha256.Sum256(append(body, []byte(secret)...))
	return hex.EncodeToString(mac[:])
}
