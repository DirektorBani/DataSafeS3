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
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/buildinfo"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) bucketEncryptionView(rec metadata.BucketRecord, canEdit bool) map[string]any {
	allow := strings.TrimSpace(os.Getenv("STORAGE_KMS_KEY_ALLOWLIST"))
	if allow == "" {
		allow = "datasafe-*"
	}
	return map[string]any{
		"enabled":               rec.DefaultEncryption != "",
		"algorithm":             rec.DefaultEncryption,
		"provider":              rec.EncryptionProvider,
		"key_id":                rec.KMSMasterKeyID,
		"key_status":            rec.KMSKeyStatus,
		"visibility":            rec.Visibility,
		"server_key_configured": s.svc.LocalKMSConfigured(),
		"vault_configured":      false,
		"allowlist":             allow,
		"can_edit":              canEdit,
		"default_key_id":        s3DefaultKey(rec),
	}
}

func s3DefaultKey(rec metadata.BucketRecord) string {
	tenant := rec.TenantID
	if tenant == "" {
		tenant = "default"
	}
	return "datasafe-" + sanitizeKeyPart(tenant) + "-" + sanitizeKeyPart(rec.Name)
}

func sanitizeKeyPart(in string) string {
	in = strings.ToLower(strings.TrimSpace(in))
	if in == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range in {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	return out
}

func (s *Server) handleGetBucketEncryption(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, s.bucketEncryptionView(rec, s.isBucketAdmin(info, rec)))
}

func (s *Server) handlePutBucketEncryption(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil || !s.isBucketAdmin(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	var req struct {
		Enabled  *bool  `json:"enabled"`
		Provider string `json:"provider"`
		KeyID    string `json:"key_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		s.finishEncryptionDisable(w, r, info, rec)
		return
	}
	if !s.requireBucketAdminFactor(w, r, "encryption_enable", rec.Name) {
		return
	}
	if strings.EqualFold(req.Provider, "vault") || strings.EqualFold(req.Provider, "vault-transit") {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "vault_not_configured",
			"message": "Сначала подключите Vault в Настройки → Vault (KMS)",
		})
		return
	}
	updated, err := s.svc.SetBucketEncryption(rec, "AES256", req.KeyID)
	if err != nil {
		writeEncryptionErr(w, err)
		return
	}
	s.logActivity(r, "bucket.encryption_changed", "bucket", rec.Name)
	writeJSON(w, http.StatusOK, s.bucketEncryptionView(updated, true))
}

func (s *Server) handleDeleteBucketEncryption(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil || !s.isBucketAdmin(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	s.finishEncryptionDisable(w, r, info, rec)
}

func (s *Server) finishEncryptionDisable(w http.ResponseWriter, r *http.Request, info auth.TokenInfo, rec metadata.BucketRecord) {
	if !s.allowDangerous(w, r, auth.ActionEncryptionDisable, rec.Name) {
		return
	}
	updated, err := s.svc.ClearBucketEncryption(rec)
	if err != nil {
		writeEncryptionErr(w, err)
		return
	}
	s.logActivity(r, "bucket.encryption_changed", "bucket", rec.Name)
	_ = info
	writeJSON(w, http.StatusOK, s.bucketEncryptionView(updated, true))
}

func (s *Server) runKMSHeartbeat(ctx context.Context) {
	node := strings.TrimSpace(s.cfg.NodeID)
	if node == "" {
		node = strings.TrimSpace(os.Getenv("STORAGE_NODE_ID"))
	}
	if node == "" {
		node = "local"
	}
	beat := func() {
		_ = s.meta.UpsertNodeHeartbeat(node, buildinfo.Version, time.Now().UTC())
	}
	beat()
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			beat()
		}
	}
}

func writeEncryptionErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, s3.ErrSSEKeyMissing):
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "server_key_missing",
			"message": "Задайте STORAGE_SSE_MASTER_KEY, чтобы включить ключ сервера.",
		})
	case errors.Is(err, s3.ErrPublicKMS):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "public_kms_conflict", "message": s3.MsgPublicKMS})
	case errors.Is(err, s3.ErrClusterUpgrade):
		writeJSON(w, http.StatusConflict, map[string]any{"error": "cluster_upgrade", "message": s3.MsgClusterUpgrade})
	case errors.Is(err, s3.ErrKeyNotAllowed):
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key_not_allowed", "message": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
}
