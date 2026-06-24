package api

import (
	"net/http"

	"github.com/DirektorBani/datasafe/internal/security"
)

func (s *Server) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"weak_secrets": security.WeakEnvVars(),
		"doc":          security.JWTSecretsDocPath(),
	})
}
