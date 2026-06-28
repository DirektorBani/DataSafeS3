package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) handleListBucketAccess(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant")
	logicalBucket := r.PathValue("bucket")
	s.serveListBucketAccess(w, r, logicalBucket, tenantID)
}

func (s *Server) handleListBucketAccessByBucket(w http.ResponseWriter, r *http.Request) {
	logicalBucket := r.PathValue("bucket")
	s.serveListBucketAccess(w, r, logicalBucket, "")
}

func (s *Server) serveListBucketAccess(w http.ResponseWriter, r *http.Request, logicalBucket, tenantID string) {
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForAccessMgmt(info, logicalBucket, tenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	if !s.canManageBucketGrants(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if !s.canAccessBucket(info, logicalBucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	out, err := s.listBucketAccessGrants(rec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	prefixOut, err := s.listBucketPrefixAccessGrants(rec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"grants":        out,
		"prefix_grants": prefixOut,
		"bucket":        logicalBucket,
		"tenant_id":     rec.EffectiveTenantID(),
	})
}

func (s *Server) handlePutBucketAccess(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant")
	logicalBucket := r.PathValue("bucket")
	s.servePutBucketAccess(w, r, logicalBucket, tenantID)
}

func (s *Server) handlePutBucketAccessByBucket(w http.ResponseWriter, r *http.Request) {
	logicalBucket := r.PathValue("bucket")
	s.servePutBucketAccess(w, r, logicalBucket, "")
}

func (s *Server) servePutBucketAccess(w http.ResponseWriter, r *http.Request, logicalBucket, tenantID string) {
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForAccessMgmt(info, logicalBucket, tenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	if !s.canManageBucketGrants(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if !s.canWriteBucket(info, logicalBucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	var req struct {
		Grants []struct {
			UserID   string `json:"user_id"`
			CanRead  bool   `json:"can_read"`
			CanWrite bool   `json:"can_write"`
		} `json:"grants"`
		PrefixGrants []struct {
			UserID   string `json:"user_id"`
			Prefix   string `json:"prefix"`
			CanRead  bool   `json:"can_read"`
			CanWrite bool   `json:"can_write"`
		} `json:"prefix_grants"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	bucketKey := rec.EffectiveStorageKey()
	oldGrants, _ := s.listBucketAccessGrants(rec)
	oldPrefixGrants, _ := s.listBucketPrefixAccessGrants(rec)

	var grants []metadata.BucketAccessGrant
	for _, g := range req.Grants {
		if g.UserID == "" {
			continue
		}
		if gerr := s.granteeAllowedForBucket(rec, g.UserID, info); gerr != nil {
			writeJSON(w, grantErrorStatus(gerr), map[string]any{"error": gerr.Error()})
			return
		}
		grants = append(grants, metadata.BucketAccessGrant{
			BucketKey: bucketKey,
			UserID:    g.UserID,
			CanRead:   g.CanRead || g.CanWrite,
			CanWrite:  g.CanWrite,
		})
	}
	var prefixGrants []metadata.BucketPrefixAccessGrant
	for _, g := range req.PrefixGrants {
		if g.UserID == "" || strings.TrimSpace(g.Prefix) == "" {
			continue
		}
		if gerr := s.granteeAllowedForBucket(rec, g.UserID, info); gerr != nil {
			writeJSON(w, grantErrorStatus(gerr), map[string]any{"error": gerr.Error()})
			return
		}
		prefixGrants = append(prefixGrants, metadata.BucketPrefixAccessGrant{
			BucketKey: bucketKey,
			UserID:    g.UserID,
			Prefix:    metadata.NormalizeSharePrefix(g.Prefix),
			CanRead:   g.CanRead || g.CanWrite,
			CanWrite:  g.CanWrite,
		})
	}
	if err := s.replaceBucketAccessGrants(rec, grants); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.replaceBucketPrefixAccessGrants(rec, prefixGrants); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.notifyBucketSharedDiff(info, logicalBucket, oldGrants, oldPrefixGrants, grants, prefixGrants)
	s.logActivity(r, metadata.ActionSettingsChanged, "bucket_access", logicalBucket)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "grants": len(grants), "prefix_grants": len(prefixGrants)})
}

func (s *Server) handleDeleteBucketAccess(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("tenant")
	logicalBucket := r.PathValue("bucket")
	userID := r.PathValue("user_id")
	s.serveDeleteBucketAccess(w, r, logicalBucket, tenantID, userID)
}

func (s *Server) handleDeleteBucketAccessByBucket(w http.ResponseWriter, r *http.Request) {
	logicalBucket := r.PathValue("bucket")
	userID := r.PathValue("user_id")
	s.serveDeleteBucketAccess(w, r, logicalBucket, "", userID)
}

func (s *Server) serveDeleteBucketAccess(w http.ResponseWriter, r *http.Request, logicalBucket, tenantID, userID string) {
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForAccessMgmt(info, logicalBucket, tenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !s.canManageBucketGrants(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if !s.canWriteBucket(info, logicalBucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	_ = s.meta.DeleteBucketAccessGrant(rec.EffectiveStorageKey(), userID)
	_ = s.meta.DeleteBucketPrefixAccessGrantsForUser(rec.EffectiveStorageKey(), userID)
	s.logActivity(r, metadata.ActionSettingsChanged, "bucket_access", logicalBucket)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resolveBucketForAccessMgmt(info auth.TokenInfo, logicalBucket, tenantID string) (metadata.BucketRecord, error) {
	if tenantID != "" {
		rec, err := s.meta.ResolveBucket(metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantID}, logicalBucket)
		if err != nil {
			return rec, err
		}
		if rec.TenantID != tenantID {
			return rec, metadata.ErrNotFound
		}
		return rec, nil
	}
	return s.resolveBucketForUser(info, logicalBucket)
}

func (s *Server) handleShareableUsers(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	logicalBucket := strings.TrimSpace(r.URL.Query().Get("bucket"))
	if logicalBucket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bucket query required"})
		return
	}
	rec, err := s.resolveBucketForUser(info, logicalBucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "bucket not found"})
		return
	}
	if !s.canManageBucketGrants(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	ownerID := rec.OwnerID
	if ownerID == "" && rec.Owner != "" {
		if u, uerr := s.meta.GetUserByUsername(rec.Owner); uerr == nil {
			ownerID = u.ID
		}
	}
	users, err := s.shareableUsersForBucket(rec, ownerID, r.URL.Query().Get("q"), 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users, "bucket": logicalBucket})
}
