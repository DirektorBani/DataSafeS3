package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/inventory"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

const inventoryMaxObjects = inventory.DefaultMaxObjects

func (s *Server) handleCreateInventoryJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Bucket     string `json:"bucket"`
		Prefix     string `json:"prefix"`
		Format     string `json:"format"`
		Schedule   string `json:"schedule"`
		DestBucket string `json:"dest_bucket"`
		DestKey    string `json:"dest_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req.Bucket = strings.TrimSpace(req.Bucket)
	req.Prefix = strings.TrimSpace(req.Prefix)
	req.Format = strings.TrimSpace(req.Format)
	req.Schedule = strings.ToLower(strings.TrimSpace(req.Schedule))
	if req.Format == "" {
		req.Format = "csv"
	}
	if req.Schedule != "" && req.Schedule != "manual" {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"error": "scheduled/cron inventory is not available in this release (manual only)",
		})
		return
	}
	if err := inventory.ValidateExportRequest(req.Bucket, req.Format); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, req.Bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.resolveBucketForUser(info, req.Bucket); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	if req.DestBucket != "" {
		if !s.canWriteBucket(info, req.DestBucket) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden dest_bucket"})
			return
		}
		if req.DestKey == "" {
			req.DestKey = fmt.Sprintf("inventory/%s/%s.csv", req.Bucket, time.Now().UTC().Format("20060102T150405Z"))
		}
	}

	job := s.invJobs.Create(req.Bucket, req.Prefix, req.Format, req.DestBucket, req.DestKey)
	s.logActivity(r, metadata.ActionInventoryExported, "bucket", req.Bucket)

	// Manual jobs run synchronously so Bolt/Postgres operators get an immediate downloadable result.
	s.runInventoryJob(r, job.ID)

	rec, ok := s.invJobs.Get(job.ID)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "job lost"})
		return
	}
	writeJSON(w, http.StatusCreated, rec.Job)
}

func (s *Server) handleGetInventoryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, ok := s.invJobs.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, rec.Job)
}

func (s *Server) handleDownloadInventoryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rec, ok := s.invJobs.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if rec.Job.Status != inventory.StatusCompleted {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "job not completed", "status": rec.Job.Status})
		return
	}
	if len(rec.CSV) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no csv payload (dest-only or expired)"})
		return
	}
	filename := fmt.Sprintf("inventory-%s.csv", rec.Job.Bucket)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(rec.CSV)
}

func (s *Server) runInventoryJob(r *http.Request, id string) {
	s.invJobs.Update(id, func(rec *inventory.JobRecord) {
		rec.Job.Status = inventory.StatusRunning
	})

	rec, ok := s.invJobs.Get(id)
	if !ok {
		return
	}
	job := rec.Job

	brec, err := s.meta.GetBucket(job.Bucket)
	if err != nil {
		s.failInventoryJob(id, err.Error())
		return
	}
	storageKey := brec.EffectiveStorageKey()
	lock := inventory.BucketLockMeta{
		ObjectLock:    brec.ObjectLock,
		RetentionDays: brec.RetentionDays,
		RetentionMode: brec.RetentionMode,
	}

	var objs []inventory.ObjectMeta
	startAfter := ""
	truncated := false
	for {
		page, hasMore, next, err := s.meta.ListObjectsPage(storageKey, job.Prefix, startAfter, 1000)
		if err != nil {
			s.failInventoryJob(id, err.Error())
			return
		}
		for _, o := range page {
			if o.IsDeleteMarker {
				continue
			}
			objs = append(objs, inventory.ObjectMeta{
				Key:            o.Key,
				Size:           o.Size,
				LastModified:   o.LastModified,
				StorageClass:   o.StorageClass,
				VersionID:      o.VersionID,
				LegalHold:      o.LegalHold,
				RetentionUntil: o.RetentionUntil,
			})
			if len(objs) >= inventoryMaxObjects {
				truncated = true
				break
			}
		}
		if truncated || !hasMore {
			break
		}
		startAfter = next
		if startAfter == "" {
			break
		}
	}

	rows := inventory.BuildRows(job.Bucket, lock, objs)
	var buf bytes.Buffer
	if err := inventory.WriteCSV(&buf, rows); err != nil {
		s.failInventoryJob(id, err.Error())
		return
	}
	csvBytes := buf.Bytes()

	if job.DestBucket != "" && job.DestKey != "" {
		destRec, err := s.meta.GetBucket(job.DestBucket)
		if err != nil {
			s.failInventoryJob(id, "dest_bucket: "+err.Error())
			return
		}
		destKey := destRec.EffectiveStorageKey()
		_, err = s.svc.PutObject(r.Context(), destKey, job.DestKey, bytes.NewReader(csvBytes), int64(len(csvBytes)), "text/csv", nil)
		if err != nil {
			s.failInventoryJob(id, "dest write: "+err.Error())
			return
		}
	}

	now := time.Now().UTC()
	s.invJobs.Update(id, func(rec *inventory.JobRecord) {
		rec.CSV = csvBytes
		rec.Job.Status = inventory.StatusCompleted
		rec.Job.ObjectCount = len(rows)
		rec.Job.Truncated = truncated
		rec.Job.FinishedAt = &now
		rec.Job.Error = ""
	})
}

func (s *Server) failInventoryJob(id, msg string) {
	now := time.Now().UTC()
	s.invJobs.Update(id, func(rec *inventory.JobRecord) {
		rec.Job.Status = inventory.StatusFailed
		rec.Job.Error = msg
		rec.Job.FinishedAt = &now
	})
}
