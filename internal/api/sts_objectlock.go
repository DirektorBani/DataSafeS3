package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

func (s *Server) handleAssumeRole(w http.ResponseWriter, r *http.Request) {
	info, ok := authFrom(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	var req struct {
		RoleArn         string `json:"role_arn"`
		RoleSessionName string `json:"role_session_name"`
		DurationSeconds int    `json:"duration_seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 3600
	}
	if req.DurationSeconds > 43200 {
		req.DurationSeconds = 43200
	}
	sessionName := strings.TrimSpace(req.RoleSessionName)
	if sessionName == "" {
		sessionName = info.Username
	}
	accessKey := "ASIA" + randomHexSTS(8)
	secretKey := randomHexSTS(32)
	sessionToken := "FQoD" + randomHexSTS(16)
	expiration := time.Now().UTC().Add(time.Duration(req.DurationSeconds) * time.Second)
	_ = s.meta.PutAccessKey(metadata.AccessKeyRecord{
		AccessKey:    accessKey,
		SecretKey:    secretKey,
		SessionToken: sessionToken,
		ExpiresAt:    &expiration,
		Label:        "sts:" + sessionName,
		Owner:        info.Username,
		OwnerID:      info.UserID,
		CreatedAt:    time.Now().UTC(),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"credentials": map[string]any{
			"access_key_id":     accessKey,
			"secret_access_key": secretKey,
			"session_token":     sessionToken,
			"expiration":        expiration.Format(time.RFC3339),
		},
		"assumed_role_user": map[string]any{
			"arn":             req.RoleArn,
			"assumed_role_id": accessKey,
		},
	})
}

func randomHexSTS(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handlePutObjectRetention(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	var req struct {
		Key         string `json:"key"`
		VersionID   string `json:"version_id"`
		Mode        string `json:"mode"` // GOVERNANCE | COMPLIANCE
		RetainUntil string `json:"retain_until"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key and retain_until required"})
		return
	}
	until, err := time.Parse(time.RFC3339, req.RetainUntil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid retain_until"})
		return
	}
	if err := s.meta.SetObjectRetention(bucket, req.Key, req.VersionID, until); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	brec, _ := s.meta.GetBucket(bucket)
	if req.Mode != "" {
		brec.ObjectLock = true
		_ = s.meta.UpdateBucket(brec)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bucket": bucket, "key": req.Key, "mode": req.Mode, "retain_until": until.UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleGetObjectRetention(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	versionID := r.URL.Query().Get("version_id")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	obj, err := s.meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	var until string
	if obj.RetentionUntil != nil {
		until = obj.RetentionUntil.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"legal_hold": obj.LegalHold, "retain_until": until,
	})
}

// extendedStats feeds Prometheus extended gauges.
func (s *Server) extendedStats() observability.ExtendedStats {
	objects, _ := s.meta.CountObjects()
	buckets, _ := s.meta.ListBuckets()
	pending, _, _ := s.meta.CountPendingReplicationTasks()
	multipart, _ := s.meta.ListMultipart("")
	var webhookFails int
	deliveries := 0
	hooks, _ := s.meta.ListWebhooks()
	for _, h := range hooks {
		ds, _ := s.meta.ListWebhookDeliveries(h.ID, 100)
		for _, d := range ds {
			deliveries++
			if !d.Success {
				webhookFails++
			}
		}
	}
	perBucket := map[string]int64{}
	perObjects := map[string]int{}
	perTenant := map[string]int64{}
	for _, b := range buckets {
		cnt, _ := s.meta.BucketObjectCount(b.EffectiveStorageKey())
		perObjects[b.Name] = cnt
		sz, _ := s.meta.BucketTotalSize(b.EffectiveStorageKey())
		perBucket[b.Name] = sz
		tid := b.TenantID
		if tid == "" {
			tid = metadata.DefaultTenantID
		}
		perTenant[tid] += sz
	}
	var versions int
	for _, b := range buckets {
		vers, _ := s.meta.ListObjectVersions(b.Name, "", 0)
		versions += len(vers)
	}
	return observability.ExtendedStats{
		ObjectsTotal:     objects,
		VersionsTotal:    versions,
		ReplicationQueue: pending,
		WebhookFailures:  webhookFails,
		MultipartActive:  len(multipart),
		ObjectsPerBucket: perObjects,
		StoragePerBucket: perBucket,
		StoragePerTenant: perTenant,
	}
}
