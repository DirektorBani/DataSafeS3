package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

var errKMSJobActive = errors.New("kms job already running")

func (s *Server) notePresignDecrypt(ctx context.Context, user, ip, bucket, key, keyID, tenant, result string, keyVersion int) {
	resource := bucket + "/" + key
	if keyID != "" {
		resource += " key_id=" + keyID + " key_version=" + strconv.Itoa(keyVersion)
	}
	if result == "key_destroyed" {
		resource += " result=key_destroyed"
	}
	s.logActivityAs(user, ip, metadata.ActionObjectDownloaded, "object", resource)
	payload := map[string]any{
		"actor": user, "access_key": user, "client_ip": ip,
		"bucket": bucket, "key": key, "result": result,
	}
	if keyID != "" {
		payload["key_id"] = keyID
		payload["key_version"] = keyVersion
	}
	s.publishEvent(ctx, "ShareDownloaded", bucket, key, "", tenant, payload)
}

func (s *Server) activeKMSJob(bucketKey string) (metadata.KMSJob, bool) {
	jobs, err := s.meta.ListKMSJobs()
	if err != nil {
		return metadata.KMSJob{}, false
	}
	var best metadata.KMSJob
	found := false
	for _, job := range jobs {
		if job.BucketKey != bucketKey {
			continue
		}
		if job.Status != kms.JobPending && job.Status != kms.JobRunning {
			continue
		}
		if !found || job.UpdatedAt.After(best.UpdatedAt) {
			best = job
			found = true
		}
	}
	return best, found
}

func (s *Server) activeKMSJobID(bucketKey string) string {
	job, ok := s.activeKMSJob(bucketKey)
	if !ok {
		return ""
	}
	return job.ID
}

func (s *Server) rejectClusterUpgrade(w http.ResponseWriter) bool {
	if s.svc != nil && s.svc.EncryptionChangesBlocked() {
		writeEncryptionErr(w, s3.ErrClusterUpgrade)
		return true
	}
	return false
}

func (s *Server) kmsJobView(job metadata.KMSJob) map[string]any {
	return map[string]any{
		"id":             job.ID,
		"type":           job.Type,
		"bucket":         job.Bucket,
		"bucket_key":     job.BucketKey,
		"key_id":         job.KeyID,
		"target_version": job.TargetVersion,
		"cursor":         job.Cursor,
		"status":         job.Status,
		"total":          job.Total,
		"done":           job.Done,
		"errors":         job.Errors,
		"samples":        job.Samples,
		"eta_seconds":    kmsJobETA(job),
		"started_at":     job.StartedAt,
		"updated_at":     job.UpdatedAt,
		"finished_at":    job.FinishedAt,
		"holder_id":      job.HolderID,
	}
}

func kmsJobETA(job metadata.KMSJob) int {
	if job.Status == kms.JobCompleted || job.Status == kms.JobFailed {
		return 0
	}
	if job.Done <= 0 || job.Total <= job.Done || job.StartedAt.IsZero() {
		return 0
	}
	elapsed := time.Since(job.StartedAt).Seconds()
	if elapsed <= 0 {
		return 0
	}
	remain := elapsed / float64(job.Done) * float64(job.Total-job.Done)
	if remain < 1 {
		return 1
	}
	return int(remain + 0.5)
}

func (s *Server) handleRotateKMSKey(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	key, err := s.meta.GetKMSKey(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !s.canRotateKey(w, r, key) {
		return
	}
	if !s.allowDangerous(w, r, auth.ActionKeyRotate, key.ID) {
		return
	}
	if s.rejectClusterUpgrade(w) {
		return
	}
	if key.Status == "destroyed" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "key_destroyed", "message": s3.MsgKeyDestroyed})
		return
	}
	if key.Provider == kms.ProviderVault {
		v := s.svc.VaultKMS()
		if v == nil {
			writeEncryptionErr(w, s3.ErrKMSUnavailable)
			return
		}
		ver, rerr := v.Rotate(r.Context(), key.ID)
		if rerr != nil {
			writeEncryptionErr(w, s3.ErrKMSUnavailable)
			return
		}
		if ver < 1 {
			ver = key.Version + 1
		}
		key.Version = ver
	} else {
		if s.svc.Local == nil {
			writeEncryptionErr(w, s3.ErrSSEKeyMissing)
			return
		}
		if key.Version < 1 {
			key.Version = 1
		}
		key.Version++
	}
	key.Generation++
	key.UpdatedAt = time.Now().UTC()
	if err := s.meta.PutKMSKey(key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionKMSKeyRotated, "kms_key", key.ID+" key_version="+strconv.Itoa(key.Version))
	writeJSON(w, http.StatusOK, map[string]any{
		"id": key.ID, "key_version": key.Version, "generation": key.Generation, "status": key.Status,
	})
}

func (s *Server) canRotateKey(w http.ResponseWriter, r *http.Request, key metadata.KMSKeyRecord) bool {
	info, _ := authFrom(r)
	if key.BucketKey != "" {
		rec, err := s.meta.GetBucketByKey(key.BucketKey)
		if err != nil || !s.isBucketAdmin(info, rec) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
			return false
		}
		return true
	}
	if !auth.IsAdmin(info.Role) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return false
	}
	return true
}

func (s *Server) handleRewrapBucket(w http.ResponseWriter, r *http.Request) {
	info, rec, ok := s.bucketAdminOr403(w, r)
	if !ok {
		return
	}
	if s.rejectClusterUpgrade(w) {
		return
	}
	if rec.DefaultEncryption == "" || rec.KMSMasterKeyID == "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "encryption_off"})
		return
	}
	if rec.KMSKeyStatus == "destroyed" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "key_destroyed", "message": s3.MsgKeyDestroyed})
		return
	}
	key, err := s.meta.GetKMSKey(rec.KMSMasterKeyID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "key_missing"})
		return
	}
	if key.Status == "destroyed" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "key_destroyed", "message": s3.MsgKeyDestroyed})
		return
	}
	job, err := s.startKMSJob(rec, key, kms.JobEncryptExisting)
	if errors.Is(err, errKMSJobActive) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "job_active", "job_id": job.ID})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = info
	writeJSON(w, http.StatusAccepted, s.kmsJobView(job))
}

func (s *Server) handleDestroyBucketKey(w http.ResponseWriter, r *http.Request) {
	_, rec, ok := s.bucketAdminOr403(w, r)
	if !ok {
		return
	}
	var req struct {
		Confirm string `json:"confirm"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Confirm != rec.Name {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "confirm_mismatch",
			"message": "type the bucket name to confirm",
		})
		return
	}
	if !s.allowDangerous(w, r, auth.ActionCryptoShred, rec.Name) {
		return
	}
	if s.rejectClusterUpgrade(w) {
		return
	}
	if rec.KMSMasterKeyID == "" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "encryption_off"})
		return
	}
	key, err := s.meta.GetKMSKey(rec.KMSMasterKeyID)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "key_missing"})
		return
	}
	if key.Status == "destroyed" || rec.KMSKeyStatus == "destroyed" {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "key_destroyed", "message": s3.MsgKeyDestroyed})
		return
	}
	if key.Provider == kms.ProviderVault {
		if v := s.svc.VaultKMS(); v != nil {
			_ = v.Destroy(r.Context(), key.ID)
		}
	}
	key.Status = "destroyed"
	key.Generation++
	key.UpdatedAt = time.Now().UTC()
	if err := s.meta.PutKMSKey(key); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	enc := metadata.BucketEncryption{
		StorageKey: rec.EffectiveStorageKey(),
		Algorithm:  rec.DefaultEncryption,
		KeyID:      key.ID,
		Provider:   rec.EncryptionProvider,
		Status:     "destroyed",
	}
	if enc.Algorithm == "" {
		enc.Algorithm = "AES256"
	}
	if err := s.meta.PutBucketEncryption(rec.EffectiveStorageKey(), enc); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	job, err := s.startKMSJob(rec, key, kms.JobCryptoShred)
	if err != nil && !errors.Is(err, errKMSJobActive) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionKMSKeyDestroyed, "kms_key", key.ID)
	body := s.kmsJobView(job)
	body["key_status"] = "destroyed"
	writeJSON(w, http.StatusAccepted, body)
}

func (s *Server) bucketAdminOr403(w http.ResponseWriter, r *http.Request) (auth.TokenInfo, metadata.BucketRecord, bool) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil || !s.isBucketAdmin(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return info, rec, false
	}
	return info, rec, true
}

func (s *Server) startKMSJob(rec metadata.BucketRecord, key metadata.KMSKeyRecord, jobType string) (metadata.KMSJob, error) {
	if existing, ok := s.activeKMSJob(rec.EffectiveStorageKey()); ok {
		return existing, errKMSJobActive
	}
	job := metadata.KMSJob{
		ID:            newKMSJobID(),
		Type:          jobType,
		BucketKey:     rec.EffectiveStorageKey(),
		Bucket:        rec.Name,
		KeyID:         key.ID,
		TargetVersion: key.Version,
		Status:        kms.JobPending,
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.meta.PutKMSJob(job); err != nil {
		return job, err
	}
	return job, nil
}

func (s *Server) handleListKMSJobs(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	jobs, err := s.meta.ListKMSJobs()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	bucket := r.URL.Query().Get("bucket")
	out := make([]map[string]any, 0, len(jobs))
	for _, job := range jobs {
		if bucket != "" && job.Bucket != bucket {
			continue
		}
		if !s.canSeeKMSJob(info, job) {
			continue
		}
		out = append(out, s.kmsJobView(job))
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": out})
}

func (s *Server) handleGetKMSJob(w http.ResponseWriter, r *http.Request) {
	job, ok := s.kmsJobForCaller(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.kmsJobView(job))
}

func (s *Server) handleKMSJobReport(w http.ResponseWriter, r *http.Request) {
	job, ok := s.kmsJobForCaller(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="kms-job-`+job.ID+`.json"`)
	writeJSON(w, http.StatusOK, s.kmsJobView(job))
}

func (s *Server) kmsJobForCaller(w http.ResponseWriter, r *http.Request) (metadata.KMSJob, bool) {
	info, _ := authFrom(r)
	job, err := s.meta.GetKMSJob(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return job, false
	}
	if !s.canSeeKMSJob(info, job) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return job, false
	}
	return job, true
}

func (s *Server) canSeeKMSJob(info auth.TokenInfo, job metadata.KMSJob) bool {
	if auth.IsAdmin(info.Role) {
		return true
	}
	rec, err := s.meta.GetBucketByKey(job.BucketKey)
	if err != nil {
		rec, err = s.meta.GetBucket(job.Bucket)
	}
	if err != nil {
		return false
	}
	return s.canAccessBucket(info, rec.Name)
}
