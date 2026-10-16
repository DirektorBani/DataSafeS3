package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/buildinfo"
	"github.com/DirektorBani/datasafe/internal/openapi"
)

// handleCapabilities serves GET /api/v1/capabilities.
//
// Unauthenticated callers receive the API version and login methods only.
// features, clients.desktop, and deprecations are returned only after a
// valid bearer token. The body never includes IdP addresses, usernames, or secrets.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	info, ok, err := s.bearerIdentity(r)
	if err != nil {
		legacy := "unauthorized"
		if errors.Is(err, errSessionExpired) {
			legacy = "session expired"
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": legacy})
		return
	}
	_ = info
	body := map[string]any{
		"api":            "v1",
		"server_version": s3.HeartbeatVersion(buildinfo.Version),
		"spec_version":   openapi.SpecVersion(),
		"login_methods":  s.loginMethods(),
	}
	if ok {
		w.Header().Set("Cache-Control", "private, max-age=300")
		body["features"] = s.capabilityFeatures()
		body["clients"] = desktopClientCompat()
		body["deprecations"] = deprecationsForCapabilities()
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	writeJSON(w, http.StatusOK, body)
}

var errSessionExpired = errors.New("session expired")

func (s *Server) bearerIdentity(r *http.Request) (auth.TokenInfo, bool, error) {
	authz := r.Header.Get("Authorization")
	if authz == "" {
		return auth.TokenInfo{}, false, nil
	}
	if !strings.HasPrefix(authz, "Bearer ") {
		return auth.TokenInfo{}, false, errors.New("unauthorized")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	if token == "" {
		return auth.TokenInfo{}, false, errors.New("unauthorized")
	}
	var info auth.TokenInfo
	var err error
	if strings.HasPrefix(token, "ds_") {
		info, err = s.validateConsoleToken(token)
	} else {
		info, err = s.jwt.Validate(token)
	}
	if err != nil {
		return auth.TokenInfo{}, false, err
	}
	if info.AuthSource == auth.AuthSourceOIDC && info.SessionID != "" {
		active, sessErr := s.validateOIDCSession(r.Context(), info.SessionID)
		if sessErr != nil || !active {
			return auth.TokenInfo{}, false, errSessionExpired
		}
	}
	return info, true, nil
}

func (s *Server) loginMethods() []string {
	methods := make([]string, 0, 2)
	if localLoginEnabled() {
		methods = append(methods, "password")
	}
	cfg, err := s.meta.GetSystemConfig()
	if err == nil && cfg.OIDC.Enabled {
		methods = append(methods, "oidc")
	}
	return methods
}

func (s *Server) capabilityFeatures() map[string]int {
	features := map[string]int{
		"auth.step_up":       1,
		"objects.multipart":  1,
		"sharing.links":      1,
		"sharing.policy":     1,
		"sharing.revoke_all": 1,
		"encryption.kms":     1,
		"errors.codes":       1,
	}
	if localLoginEnabled() {
		features["auth.password"] = 1
		features["auth.totp"] = 1
	}
	cfg, err := s.meta.GetSystemConfig()
	if err == nil && cfg.OIDC.Enabled {
		features["auth.oidc"] = 1
	}
	return features
}

func desktopClientCompat() map[string]any {
	// max_tested stays null until a Desktop release has passed this server's
	// contract suite. min_required stays null until a security floor is set.
	return map[string]any{
		"desktop": map[string]any{
			"min_supported":       "1.0.0",
			"min_required":        nil,
			"min_required_reason": nil,
			"advisory_url":        nil,
			"max_tested":          nil,
		},
	}
}

// TokenForTest mints a bearer token for tests that need a role login cannot issue.
func (s *Server) TokenForTest(info auth.TokenInfo) (string, error) {
	return s.jwt.Issue(info)
}
