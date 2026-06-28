package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

// handleTransitionStorageClass updates object storage class metadata (hot → warm MVP).
func (s *Server) handleTransitionStorageClass(w http.ResponseWriter, r *http.Request) {
	if s.readOnlyGuard(w, r) {
		return
	}
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	var req struct {
		Key          string `json:"key"`
		StorageClass string `json:"storage_class"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key and storage_class required"})
		return
	}
	target := strings.ToLower(strings.TrimSpace(req.StorageClass))
	if target == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "storage_class required"})
		return
	}
	if !isAllowedTransition(target) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unsupported storage_class transition"})
		return
	}
	sk := s.storageKeyForLogicalBucket(bucket)
	rec, err := s.meta.GetObject(sk, req.Key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	from := strings.ToLower(rec.StorageClass)
	if from == "" {
		if brec, err := s.meta.GetBucket(bucket); err == nil {
			from = strings.ToLower(brec.StorageClass)
		}
	}
	if from == "" {
		from = metadata.StorageClassHot
	}
	if !transitionAllowed(from, target) {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":   "transition not allowed",
			"from":    from,
			"to":      target,
			"allowed": []string{metadata.StorageClassWarm, metadata.StorageClassCold},
		})
		return
	}
	rec.StorageClass = target
	if err := s.meta.PutObject(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionBucketUpdated, "object", bucket+"/"+req.Key)
	writeJSON(w, http.StatusOK, map[string]any{
		"bucket":        bucket,
		"key":           req.Key,
		"storage_class": target,
	})
}

func isAllowedTransition(class string) bool {
	switch class {
	case metadata.StorageClassHot, metadata.StorageClassWarm, metadata.StorageClassCold,
		metadata.StorageClassStandard, metadata.StorageClassIA, metadata.StorageClassGlacier:
		return true
	default:
		return false
	}
}

func transitionAllowed(from, to string) bool {
	from = normalizeAdminStorageClass(from)
	to = normalizeAdminStorageClass(to)
	switch from {
	case metadata.StorageClassHot:
		return to == metadata.StorageClassWarm || to == metadata.StorageClassCold
	case metadata.StorageClassWarm:
		return to == metadata.StorageClassCold
	default:
		return false
	}
}

func normalizeAdminStorageClass(class string) string {
	c := strings.ToLower(strings.TrimSpace(class))
	switch c {
	case metadata.StorageClassWarm, "standard_ia", "ia":
		return metadata.StorageClassWarm
	case metadata.StorageClassCold, "glacier":
		return metadata.StorageClassCold
	case metadata.StorageClassHot, "standard", "":
		return metadata.StorageClassHot
	default:
		return metadata.AdminStorageClassFromS3(c)
	}
}
