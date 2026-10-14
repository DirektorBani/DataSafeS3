package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/buildinfo"
	"github.com/DirektorBani/datasafe/internal/metadata"
	pgmeta "github.com/DirektorBani/datasafe/internal/metadata/postgres"
)

func (s *Server) bucketEncryptionView(rec metadata.BucketRecord, canEdit bool) map[string]any {
	allow := strings.TrimSpace(os.Getenv("STORAGE_KMS_KEY_ALLOWLIST"))
	if allow == "" {
		allow = "datasafe-*"
	}
	backend := "bolt"
	if _, ok := s.meta.(*pgmeta.Store); ok {
		backend = "postgres"
	}
	ha := s.haLeader != nil && s.haLeader.Enabled()
	return map[string]any{
		"enabled":               rec.DefaultEncryption != "",
		"algorithm":             rec.DefaultEncryption,
		"provider":              rec.EncryptionProvider,
		"key_id":                rec.KMSMasterKeyID,
		"key_status":            rec.KMSKeyStatus,
		"visibility":            rec.Visibility,
		"server_key_configured": s.svc.LocalKMSConfigured(),
		"vault_configured":      s.svc.VaultKMS() != nil,
		"kms_available":         s.kmsHealthAvailable(),
		"kms_probed":            s.kmsProbed(),
		"allowlist":             allow,
		"can_edit":              canEdit,
		"default_key_id":        s3DefaultKey(rec),
		"metadata_backend":      backend,
		"ha_enabled":            ha,
	}
}

func (s *Server) kmsHealthAvailable() bool {
	_, ok := s.kmsHealth()
	return ok
}

func (s *Server) kmsProbed() bool {
	p := s.svc.VaultKMS()
	return p != nil && p.Probed()
}

func s3DefaultKey(rec metadata.BucketRecord) string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_KMS_DEFAULT_KEY")); v != "" {
		return v
	}
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
	algo := "AES256"
	if strings.EqualFold(req.Provider, "vault") || strings.EqualFold(req.Provider, "vault-transit") {
		if s.svc.VaultKMS() == nil {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error":   "vault_not_configured",
				"message": "Сначала подключите Vault в Настройки → Vault (KMS)",
			})
			return
		}
		algo = "aws:kms"
	}
	updated, err := s.svc.SetBucketEncryption(rec, algo, req.KeyID)
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

// advertiseClusterVersion is true when this process must be visible to the
// mixed-version guard. An empty key on Bolt does not qualify.
func (s *Server) advertiseClusterVersion() bool {
	if strings.TrimSpace(s.cfg.SSEMasterKey) != "" {
		return true
	}
	if _, ok := s.meta.(*pgmeta.Store); ok {
		return true
	}
	return s.haLeader != nil && s.haLeader.Enabled()
}

func (s *Server) clusterNodeID() string {
	if s.haLeader != nil && s.haLeader.Enabled() && strings.TrimSpace(s.haLeader.NodeID()) != "" {
		return s.haLeader.NodeID()
	}
	node := strings.TrimSpace(s.cfg.NodeID)
	if node == "" {
		node = strings.TrimSpace(os.Getenv("STORAGE_NODE_ID"))
	}
	if node == "" {
		return "local"
	}
	return node
}

func (s *Server) writeClusterHeartbeat() {
	id := s.clusterNodeID()
	_ = s.meta.UpsertNodeHeartbeat(id, s3.HeartbeatVersion(buildinfo.Version), time.Now().UTC())
	if s.haLeader != nil && s.haLeader.Enabled() {
		if addr := advertiseURL(); addr != "" {
			_ = s.meta.RegisterNodeAddress(id, addr)
		}
	}
}

// advertiseURL is the address peers dial for this process. STORAGE_ADVERTISE_URL
// wins. Otherwise the hostname and STORAGE_ADDR port are registered so a
// follower can forward writes without a hand-written cluster.nodes list.
func advertiseURL() string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_ADVERTISE_URL")); v != "" {
		return v
	}
	port := "9000"
	if addr := strings.TrimSpace(os.Getenv("STORAGE_ADDR")); addr != "" {
		if _, p, err := net.SplitHostPort(addr); err == nil && p != "" {
			port = p
		} else if strings.HasPrefix(addr, ":") && len(addr) > 1 {
			port = addr[1:]
		}
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return ""
	}
	return "http://" + host + ":" + port
}

func (s *Server) runKMSHeartbeat(ctx context.Context) {
	beat := func() {
		s.writeClusterHeartbeat()
		s.refreshVaultFromMeta()
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
	case errors.Is(err, s3.ErrKMSUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":   "KMS unavailable",
			"message": s3.MsgKMSDown,
		})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
}
