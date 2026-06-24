package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func splitFolderListing(objs []metadata.ObjectRecord, prefix string) (folders []string, files []metadata.ObjectRecord) {
	seen := map[string]bool{}
	for _, o := range objs {
		rest := o.Key
		if prefix != "" {
			if !strings.HasPrefix(rest, prefix) {
				continue
			}
			rest = strings.TrimPrefix(rest, prefix)
		}
		if rest == "" {
			continue
		}
		if idx := strings.Index(rest, "/"); idx >= 0 {
			folder := prefix + rest[:idx+1]
			if !seen[folder] {
				seen[folder] = true
				folders = append(folders, folder)
			}
			continue
		}
		files = append(files, o)
	}
	sort.Strings(folders)
	return folders, files
}

// paginateDelimitedListing builds folder/file entries at the current prefix level,
// then paginates that virtual listing. Listing all objects first avoids hiding
// root-level files when nested keys fill the object page.
func paginateDelimitedListing(objs []metadata.ObjectRecord, prefix, startAfter string, maxKeys int) (folders []string, files []metadata.ObjectRecord, truncated bool, nextMarker string) {
	allFolders, allFiles := splitFolderListing(objs, prefix)
	folderSet := map[string]bool{}
	for _, f := range allFolders {
		folderSet[f] = true
	}
	fileMap := map[string]metadata.ObjectRecord{}
	for _, f := range allFiles {
		fileMap[f.Key] = f
	}
	var keys []string
	keys = append(keys, allFolders...)
	for _, f := range allFiles {
		keys = append(keys, f.Key)
	}
	sort.Strings(keys)
	start := 0
	if startAfter != "" {
		for i, k := range keys {
			if k > startAfter {
				start = i
				break
			}
			if i == len(keys)-1 {
				start = len(keys)
			}
		}
	}
	pageKeys := keys[start:]
	if maxKeys <= 0 {
		maxKeys = 100
	}
	truncated = len(pageKeys) > maxKeys
	if truncated {
		pageKeys = pageKeys[:maxKeys]
	}
	for _, k := range pageKeys {
		if folderSet[k] {
			folders = append(folders, k)
		} else if f, ok := fileMap[k]; ok {
			files = append(files, f)
		}
	}
	if truncated && len(pageKeys) > 0 {
		nextMarker = pageKeys[len(pageKeys)-1]
	}
	return folders, files, truncated, nextMarker
}

func (s *Server) handleListObjectVersionsJSON(w http.ResponseWriter, r *http.Request) {
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
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	prefix := r.URL.Query().Get("prefix")
	if eff, ok := s.allowedListPrefix(info, rec, prefix); !ok {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	} else {
		prefix = eff
	}
	versions, err := s.meta.ListObjectVersions(sk, prefix, 1000)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	filtered := make([]metadata.ObjectRecord, 0, len(versions))
	for _, v := range versions {
		if s.canAccessObjectKey(info, bucket, v.Key) {
			filtered = append(filtered, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"versions": filtered})
}

func (s *Server) handleDownloadObjectJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.PathValue("key")
	info, _ := authFrom(r)
	if !s.canAccessObjectKey(info, bucket, key) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	rc, rec, err := s.svc.GetObject(r.Context(), sk, key, versionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	defer rc.Close()
	s.logActivity(r, metadata.ActionObjectDownloaded, "object", bucket+"/"+key)
	if rec.ContentType != "" {
		w.Header().Set("Content-Type", rec.ContentType)
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Disposition", "attachment; filename=\""+objectFileName(key)+"\"")
	w.Header().Set("ETag", rec.ETag)
	if rec.VersionID != "" {
		w.Header().Set("X-Amz-Version-Id", rec.VersionID)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func objectFileName(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 && i < len(key)-1 {
		return key[i+1:]
	}
	return key
}

func (s *Server) handleObjectMetadataJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	info, _ := authFrom(r)
	if !s.canAccessObjectKey(info, bucket, key) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	rec, err := s.svc.HeadObject(r.Context(), sk, key, versionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": rec})
}

func (s *Server) handleObjectActionsJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Action     string `json:"action"`
		Key        string `json:"key"`
		DestBucket string `json:"dest_bucket"`
		DestKey    string `json:"dest_key"`
		VersionID  string `json:"version_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Action == "" || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "action and key required"})
		return
	}
	switch req.Action {
	case "restore":
		if req.VersionID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "version_id required"})
			return
		}
		rec, err := s.svc.RestoreObjectVersion(r.Context(), sk, req.Key, req.VersionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.logActivity(r, metadata.ActionObjectUploaded, "object", bucket+"/"+req.Key)
		writeJSON(w, http.StatusOK, map[string]any{"object": rec})
	case "copy":
		if req.DestKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dest_key required"})
			return
		}
		if req.DestBucket == "" {
			req.DestBucket = bucket
		}
		if !s.canWriteBucket(info, req.DestBucket) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
			return
		}
		destSK, ok := s.bucketKeyOr404(w, info, req.DestBucket)
		if !ok {
			return
		}
		rec, err := s.svc.CopyObject(r.Context(), sk, req.Key, destSK, req.DestKey)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.logActivity(r, metadata.ActionObjectUploaded, "object", req.DestBucket+"/"+req.DestKey)
		writeJSON(w, http.StatusOK, map[string]any{"object": rec})
	case "move", "rename":
		if req.DestKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dest_key required"})
			return
		}
		destBucket := bucket
		if req.DestBucket != "" {
			destBucket = req.DestBucket
		}
		if !s.canWriteBucket(info, destBucket) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
			return
		}
		destSK, ok := s.bucketKeyOr404(w, info, destBucket)
		if !ok {
			return
		}
		rec, err := s.svc.CopyObject(r.Context(), sk, req.Key, destSK, req.DestKey)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := s.svc.DeleteObject(r.Context(), sk, req.Key, ""); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		s.logActivity(r, metadata.ActionObjectDeleted, "object", bucket+"/"+req.Key)
		s.logActivity(r, metadata.ActionObjectUploaded, "object", destBucket+"/"+req.DestKey)
		writeJSON(w, http.StatusOK, map[string]any{"object": rec})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown action"})
	}
}

func (s *Server) handleDeleteFolderJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Prefix    string
		Recursive bool
	}
	if q := strings.TrimSpace(r.URL.Query().Get("prefix")); q != "" {
		req.Prefix = q
	}
	if strings.EqualFold(r.URL.Query().Get("recursive"), "true") || r.URL.Query().Get("recursive") == "1" {
		req.Recursive = true
	}
	var body struct {
		Prefix    string `json:"prefix"`
		Recursive bool   `json:"recursive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
		if body.Prefix != "" {
			req.Prefix = body.Prefix
		}
		if body.Recursive {
			req.Recursive = true
		}
	}
	if req.Prefix == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "prefix required"})
		return
	}
	prefix := req.Prefix
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	objs, err := s.meta.ListObjects(sk, prefix, 0)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	var contents []metadata.ObjectRecord
	for _, o := range objs {
		if o.Key == prefix && o.Size == 0 {
			continue
		}
		contents = append(contents, o)
	}
	if len(contents) > 0 && !req.Recursive {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":        "folder not empty",
			"object_count": len(contents),
		})
		return
	}
	deleted := 0
	for _, o := range objs {
		if err := s.svc.DeleteObject(r.Context(), sk, o.Key, ""); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if o.Size > 0 {
			_ = s.meta.AddUsageBytes(0, o.Size)
		}
		deleted++
		s.logActivity(r, metadata.ActionObjectDeleted, "object", bucket+"/"+o.Key)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted})
}

func (s *Server) handleCreateFolderJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	key := strings.Trim(req.Name, "/")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid folder name"})
		return
	}
	key += "/"
	rec, err := s.svc.PutObject(r.Context(), sk, key, strings.NewReader(""), 0, "application/x-directory", nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionObjectUploaded, "object", bucket+"/"+key)
	writeJSON(w, http.StatusCreated, map[string]any{"object": rec})
}

func (s *Server) handleBulkDeleteObjectsJSON(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	var req struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Keys) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "keys required"})
		return
	}
	deleted := 0
	var errors []string
	for _, key := range req.Keys {
		obj, err := s.meta.GetObject(sk, key)
		if err != nil {
			errors = append(errors, key+": not found")
			continue
		}
		if err := s.svc.DeleteObject(r.Context(), sk, key, ""); err != nil {
			errors = append(errors, key+": "+err.Error())
			continue
		}
		if obj.Size > 0 {
			_ = s.meta.AddUsageBytes(0, obj.Size)
		}
		deleted++
		s.logActivity(r, metadata.ActionObjectDeleted, "object", bucket+"/"+key)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "errors": errors})
}

func (s *Server) handleGetBucketSettingsJSON(w http.ResponseWriter, r *http.Request) {
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
	vis := rec.Visibility
	if vis == "" {
		vis = "private"
	}
	_ = s.meta.TouchRecentItem(info.UserID, bucket, "")
	writeJSON(w, http.StatusOK, map[string]any{
		"name":                rec.Name,
		"owner":               rec.Owner,
		"owner_id":            rec.OwnerID,
		"description":         rec.Description,
		"versioning_enabled":  rec.Versioning,
		"object_lock_enabled": rec.ObjectLock,
		"retention_days":      rec.RetentionDays,
		"storage_class":       rec.StorageClass,
		"tenant_id":           rec.TenantID,
		"visibility":          vis,
		"max_size_bytes":      rec.MaxSizeBytes,
		"max_objects":         rec.MaxObjects,
		"lifecycle_rules":     rec.LifecycleRules,
		"tags":                rec.Tags,
	})
}
