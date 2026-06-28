package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	filter := s.bucketListFilter(info)

	s.ensureDailySnapshot()

	buckets, err := s.meta.ListBucketsFiltered(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	bucketCount := len(buckets)

	objCount := 0
	totalSize := int64(0)
	bucketStats, err := s.meta.BucketUsageStats(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for _, bs := range bucketStats {
		objCount += bs.ObjectCount
		totalSize += bs.TotalSize
	}

	counters, _ := s.meta.GetUsageCounters()
	uploadBytes := counters.UploadBytes
	downloadBytes := counters.DownloadBytes
	if !s.usageIncludesTransferStats(info, buckets) {
		uploadBytes = 0
		downloadBytes = 0
	}
	snapshots, _ := s.meta.ListUsageSnapshots(30)
	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Date < snapshots[j].Date
	})

	quota := map[string]any{}
	if !auth.CanSeeSystemUsage(info.Role) {
		user, err := s.meta.GetUserByUsername(info.Username)
		if err == nil && (user.MaxSizeBytes > 0 || user.MaxObjects > 0) {
			usedObjects, usedBytes, _ := s.meta.OwnerUsage(info.Username)
			q := map[string]any{
				"max_size_bytes": user.MaxSizeBytes,
				"max_objects":    user.MaxObjects,
				"used_size":      usedBytes,
				"used_objects":   usedObjects,
			}
			if user.MaxSizeBytes > 0 {
				q["remaining_size"] = user.MaxSizeBytes - usedBytes
			}
			if user.MaxObjects > 0 {
				q["remaining_objects"] = user.MaxObjects - int64(usedObjects)
			}
			quota = q
		}
	}

	// Filter snapshots for user role (approximate from global if no per-user tracking)
	storageSeries := make([]map[string]any, 0, len(snapshots))
	objectSeries := make([]map[string]any, 0, len(snapshots))
	for _, snap := range snapshots {
		storageSeries = append(storageSeries, map[string]any{
			"date":  snap.Date,
			"bytes": snap.StorageBytes,
		})
		objectSeries = append(objectSeries, map[string]any{
			"date":    snap.Date,
			"objects": snap.ObjectCount,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"scope": map[string]any{
			"system_wide": auth.CanSeeSystemUsage(info.Role),
		},
		"summary": map[string]any{
			"bucket_count":   bucketCount,
			"object_count":   objCount,
			"total_size":     totalSize,
			"upload_bytes":   uploadBytes,
			"download_bytes": downloadBytes,
		},
		"quota":          quota,
		"buckets":        bucketStats,
		"storage_growth": storageSeries,
		"objects_growth": objectSeries,
	})
}

func (s *Server) handleListBucketSettings(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	buckets, err := s.meta.ListBucketsFiltered(s.bucketListFilter(info))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type settingsRow struct {
		Name           string                   `json:"name"`
		Owner          string                   `json:"owner"`
		Description    string                   `json:"description"`
		Versioning     bool                     `json:"versioning_enabled"`
		ObjectLock     bool                     `json:"object_lock_enabled"`
		RetentionDays  int                      `json:"retention_days"`
		StorageClass   string                   `json:"storage_class"`
		TenantID       string                   `json:"tenant_id"`
		Visibility     string                   `json:"visibility"`
		MaxSizeBytes   int64                    `json:"max_size_bytes"`
		MaxObjects     int64                    `json:"max_objects"`
		LifecycleRules []metadata.LifecycleRule `json:"lifecycle_rules"`
	}
	var out []settingsRow
	for _, b := range buckets {
		vis := b.Visibility
		if vis == "" {
			vis = "private"
		}
		out = append(out, settingsRow{
			Name: b.Name, Owner: b.Owner, Description: b.Description,
			Versioning: b.Versioning, ObjectLock: b.ObjectLock,
			RetentionDays: b.RetentionDays, StorageClass: b.StorageClass,
			TenantID: b.TenantID, Visibility: vis,
			MaxSizeBytes: b.MaxSizeBytes, MaxObjects: b.MaxObjects,
			LifecycleRules: b.LifecycleRules,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": out})
}

func (s *Server) handleUpdateBucketSettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, name) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.resolveBucketForUser(info, name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Description    string                   `json:"description"`
		Versioning     *bool                    `json:"versioning_enabled"`
		ObjectLock     *bool                    `json:"object_lock_enabled"`
		RetentionDays  *int                     `json:"retention_days"`
		StorageClass   string                   `json:"storage_class"`
		Visibility     string                   `json:"visibility"`
		MaxSizeBytes   *int64                   `json:"max_size_bytes"`
		MaxObjects     *int64                   `json:"max_objects"`
		LifecycleRules []metadata.LifecycleRule `json:"lifecycle_rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	rec.Description = req.Description
	if req.Versioning != nil {
		rec.Versioning = *req.Versioning
	}
	if req.ObjectLock != nil {
		rec.ObjectLock = *req.ObjectLock
	}
	if req.RetentionDays != nil {
		rec.RetentionDays = *req.RetentionDays
	}
	if req.StorageClass != "" {
		rec.StorageClass = req.StorageClass
	}
	if req.Visibility != "" {
		if err := s.applyBucketVisibilityPolicy(info, name, req.Visibility); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		rec, _ = s.meta.GetBucket(name)
	}
	if req.MaxSizeBytes != nil {
		rec.MaxSizeBytes = *req.MaxSizeBytes
	}
	if req.MaxObjects != nil {
		rec.MaxObjects = *req.MaxObjects
	}
	if req.LifecycleRules != nil {
		rec.LifecycleRules = req.LifecycleRules
	}
	if err := s.meta.UpdateBucket(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "bucket", name)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bucket": rec.Name})
}
