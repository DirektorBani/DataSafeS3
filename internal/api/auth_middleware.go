package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
)

type ctxKey int

const authCtxKey ctxKey = 1

func withAuth(r *http.Request, info auth.TokenInfo) *http.Request {
	ctx := context.WithValue(r.Context(), authCtxKey, info)
	return r.WithContext(ctx)
}

func authFrom(r *http.Request) (auth.TokenInfo, bool) {
	v, ok := r.Context().Value(authCtxKey).(auth.TokenInfo)
	return v, ok
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		return host[:i]
	}
	return host
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		token := strings.TrimPrefix(authz, "Bearer ")
		var info auth.TokenInfo
		var err error
		if strings.HasPrefix(token, "ds_") {
			info, err = s.validateConsoleToken(token)
		} else {
			info, err = s.jwt.Validate(token)
		}
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		if info.AuthSource == auth.AuthSourceOIDC && info.SessionID != "" {
			active, err := s.validateOIDCSession(r.Context(), info.SessionID)
			if err != nil || !active {
				writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "session expired"})
				return
			}
		}
		if !mfaSetupExemptPath(r.URL.Path) {
			if blocked, msg := s.adminMFASetupRequired(info); blocked {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": msg, "mfa_setup_required": true})
				return
			}
		}
		next(w, withAuth(r, info))
	}
}

func mfaSetupExemptPath(path string) bool {
	exempt := []string{
		"/api/v1/me",
		"/api/v1/me/locale",
		"/api/v1/me/password",
		"/api/v1/admin/logout",
		"/api/v1/mfa/",
		"/api/v1/me/mfa/webauthn/",
		"/api/v1/setup",
	}
	for _, p := range exempt {
		if path == p || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func (s *Server) adminMFASetupRequired(info auth.TokenInfo) (bool, string) {
	if info.Role != auth.RoleAdministrator {
		return false, ""
	}
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.MFA.RequireAdminMFA {
		return false, ""
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		return false, ""
	}
	if user.MFAEnabled || user.WebAuthnCredentials != "" {
		return false, ""
	}
	return true, "mfa_setup_required"
}

func (s *Server) requireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	allowed := map[string]struct{}{}
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
			info, _ := authFrom(r)
			if _, ok := allowed[info.Role]; !ok {
				writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
				return
			}
			next(w, r)
		})
	}
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireRole(auth.RoleAdministrator)(next)
}

func (s *Server) bucketOwnerFilter(info auth.TokenInfo) string {
	if auth.CanSeeAllBuckets(info.Role) {
		return ""
	}
	return info.Username
}
