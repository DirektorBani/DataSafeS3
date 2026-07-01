package api

import (
	"net/http"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

func (s *Server) handleSecurityStatus(w http.ResponseWriter, r *http.Request) {
	registryCount := 0
	var fe *fieldenc.Service
	if reporter, ok := s.meta.(metadata.FieldEncryptionReporter); ok {
		registryCount = reporter.EncryptionRegistryCount()
		fe = reporter.FieldEnc()
	}
	if fe == nil {
		var err error
		fe, err = fieldenc.FromEnv()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"weak_secrets":      security.WeakEnvVars(),
		"doc":               security.JWTSecretsDocPath(),
		"field_encryption":  fe.Status(registryCount),
	})
}
