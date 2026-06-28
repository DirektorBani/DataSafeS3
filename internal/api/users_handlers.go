package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	wa "github.com/DirektorBani/datasafe/internal/auth/webauthn"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) handleListActivity(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	q := r.URL.Query()
	f := metadata.ActivityFilter{
		Period: q.Get("period"),
		User:   q.Get("user"),
		Action: q.Get("action"),
		Bucket: q.Get("bucket"),
		IP:     q.Get("ip"),
		Search: q.Get("search"),
	}
	if off, err := strconv.Atoi(q.Get("offset")); err == nil {
		f.Offset = off
	}
	if lim, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = lim
	}
	if !auth.CanSeeAllActivity(info.Role) {
		f.LimitUser = info.Username
	}
	result, err := s.meta.ListActivity(f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	s.logActivity(r, metadata.ActionLogout, "session", "-")

	resp := map[string]any{}
	if info.AuthSource == auth.AuthSourceOIDC && info.SessionID != "" {
		sess, ok := s.oidcSessions.Get(info.SessionID)
		s.oidcSessions.Delete(info.SessionID)
		if ok {
			cfg, err := s.meta.GetSystemConfig()
			if err == nil && cfg.OIDC.Enabled {
				issuers := s.oidcIssuers(cfg)
				postLogout := strings.TrimSpace(cfg.OIDC.RedirectURL)
				if postLogout == "" {
					postLogout = "http://localhost:8080/login"
				} else if u, err := url.Parse(postLogout); err == nil {
					u.Path = "/login"
					u.RawQuery = ""
					u.Fragment = ""
					postLogout = u.String()
				}
				if logoutURL, err := auth.BuildEndSessionURL(issuers, cfg.OIDC.ClientID, sess.IDToken, postLogout, http.DefaultClient); err == nil && logoutURL != "" {
					resp["oidc_logout_url"] = logoutURL
				}
			}
		}
	}
	if len(resp) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	s.ensureHomeBucket(info)
	rec, err := s.meta.GetUser(info.UserID)
	if err == nil {
		s.markAdminFirstLoginIfNeeded(rec)
	}
	memberships := s.userTenantMemberships(info.UserID)
	type tenantMembershipView struct {
		TenantID   string `json:"tenant_id"`
		TenantName string `json:"tenant_name,omitempty"`
		Role       string `json:"role"`
	}
	tenantViews := make([]tenantMembershipView, 0, len(memberships))
	for _, m := range memberships {
		v := tenantMembershipView{TenantID: m.TenantID, Role: m.Role}
		if t, terr := s.meta.GetTenant(m.TenantID); terr == nil {
			v.TenantName = t.Name
		}
		tenantViews = append(tenantViews, v)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"username":           info.Username,
			"role":               info.Role,
			"user_id":            info.UserID,
			"tenant_memberships": tenantViews,
			"is_tenant_admin":    auth.CanManageAnyTenant(info.Role, memberships),
		})
		return
	}
	resp := map[string]any{
		"username":           rec.Username,
		"email":              rec.Email,
		"role":               rec.Role,
		"status":             rec.Status,
		"user_id":            rec.ID,
		"tenant_id":          rec.TenantID,
		"mfa_enabled":        rec.MFAEnabled,
		"auth_source":        rec.AuthSource,
		"tenant_memberships": tenantViews,
		"is_tenant_admin":    auth.CanManageAnyTenant(info.Role, memberships),
	}
	if rec.Locale != "" {
		resp["locale"] = rec.Locale
	}
	if len(wa.ParseCredentials(rec.WebAuthnCredentials)) > 0 {
		resp["webauthn_enabled"] = true
		resp["passkey_count"] = len(wa.ParseCredentials(rec.WebAuthnCredentials))
	}
	cfg, _ := s.meta.GetSystemConfig()
	if cfg.MFA.RequireAdminMFA && rec.Role == metadata.RoleAdministrator && !rec.MFAEnabled && rec.WebAuthnCredentials == "" {
		resp["mfa_setup_required"] = true
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateLocale(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		Locale string `json:"locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	locale := strings.TrimSpace(req.Locale)
	if locale != "en" && locale != "ru" && locale != "de" && locale != "fr" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "locale must be en, ru, de, or fr"})
		return
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	user.Locale = locale
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "locale": locale})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "new password required"})
		return
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	if user.AuthSource == "ldap" || user.AuthSource == auth.AuthSourceOIDC {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "password managed by external provider"})
		return
	}
	if req.CurrentPassword == "" || !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid current password"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hash failed"})
		return
	}
	user.PasswordHash = hash
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if user.Role == metadata.RoleAdministrator {
		if cfg, err := s.meta.GetSystemConfig(); err == nil && !cfg.AdminPasswordChanged {
			cfg.AdminPasswordChanged = true
			_ = s.meta.PutSystemConfig(cfg)
		}
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "password", "changed")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	if !auth.CanManageUsers(info.Role) && !auth.CanManageAnyTenant(info.Role, s.userTenantMemberships(info.UserID)) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	users, err := s.meta.ListUsers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type safe struct {
		ID           string     `json:"id"`
		Username     string     `json:"username"`
		Email        string     `json:"email"`
		Role         string     `json:"role"`
		Status       string     `json:"status"`
		TenantID     string     `json:"tenant_id"`
		MaxSizeBytes int64      `json:"max_size_bytes"`
		MaxObjects   int64      `json:"max_objects"`
		LastLogin    *time.Time `json:"last_login,omitempty"`
		CreatedAt    time.Time  `json:"created_at"`
	}
	var out []safe
	for _, u := range users {
		out = append(out, safe{
			ID: u.ID, Username: u.Username, Email: u.Email,
			Role: u.Role, Status: u.Status, TenantID: u.TenantID,
			MaxSizeBytes: u.MaxSizeBytes, MaxObjects: u.MaxObjects,
			LastLogin: u.LastLogin, CreatedAt: u.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username     string `json:"username"`
		Email        string `json:"email"`
		Password     string `json:"password"`
		Role         string `json:"role"`
		Status       string `json:"status"`
		MaxSizeBytes int64  `json:"max_size_bytes"`
		MaxObjects   int64  `json:"max_objects"`
		TenantID     string `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
		return
	}
	if req.Role == "" {
		req.Role = metadata.RoleUser
	}
	if req.Status == "" {
		req.Status = metadata.StatusActive
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hash failed"})
		return
	}
	id := randomID()
	rec := metadata.UserRecord{
		ID: id, Username: req.Username, Email: req.Email,
		PasswordHash: hash, Role: req.Role, Status: req.Status,
		TenantID: req.TenantID, MaxSizeBytes: req.MaxSizeBytes, MaxObjects: req.MaxObjects,
		CreatedAt: time.Now().UTC(),
	}
	if rec.TenantID == "" {
		rec.TenantID = metadata.DefaultTenantID
	}
	if err := s.meta.PutUser(rec); err != nil {
		if err == metadata.ErrUserExists {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionUserCreated, "user", req.Username)
	s.emitEvent(metadata.EventUserCreated, map[string]any{"username": req.Username, "user_id": id, "role": req.Role})
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "user_id": id, "username": req.Username,
		"email": req.Email, "role": req.Role, "status": req.Status,
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetUser(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Email        string `json:"email"`
		Role         string `json:"role"`
		Status       string `json:"status"`
		MaxSizeBytes *int64 `json:"max_size_bytes"`
		MaxObjects   *int64 `json:"max_objects"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Email != "" {
		rec.Email = req.Email
	}
	if req.Role != "" {
		rec.Role = req.Role
	}
	if req.Status != "" {
		rec.Status = req.Status
	}
	if req.MaxSizeBytes != nil {
		rec.MaxSizeBytes = *req.MaxSizeBytes
	}
	if req.MaxObjects != nil {
		rec.MaxObjects = *req.MaxObjects
	}
	if err := s.meta.UpdateUser(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetUser(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if err := s.meta.DeleteUser(id); err != nil {
		if err == metadata.ErrLastAdmin {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionUserDeleted, "user", rec.Username)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, err := s.meta.GetUser(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password required"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hash failed"})
		return
	}
	rec.PasswordHash = hash
	if err := s.meta.UpdateUser(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
