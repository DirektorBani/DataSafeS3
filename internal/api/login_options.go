package api

import (
	"net/http"
	"os"
	"strings"
)

func envTruthy(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

func envFlagDefaultTrue(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return true
	}
	return envTruthy(name)
}

// localLoginEnabled: password form + POST /admin/login. Default on.
func localLoginEnabled() bool {
	return envFlagDefaultTrue("STORAGE_LOCAL_LOGIN_ENABLED")
}

// handleAuthLoginOptions exposes env feature flags for the login page (no secrets).
func (s *Server) handleAuthLoginOptions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"local_login_enabled": localLoginEnabled(),
	})
}
