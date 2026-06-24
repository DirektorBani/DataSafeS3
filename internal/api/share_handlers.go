package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) applyBucketVisibilityPolicy(info auth.TokenInfo, logicalBucket, visibility string) error {
	rec, err := s.resolveBucketForUser(info, logicalBucket)
	if err != nil {
		return err
	}
	rec.Visibility = visibility
	if visibility == "public-read" {
		rec.Policy = publicReadBucketPolicy(logicalBucket)
	} else if visibility == "private" && rec.Policy == publicReadBucketPolicy(logicalBucket) {
		rec.Policy = ""
	}
	return s.meta.UpdateBucket(rec)
}

func (s *Server) handleListSharedLinks(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	links, err := s.meta.ListSharedLinks(bucket, key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": links})
}

func (s *Server) handleCreateSharedLink(w http.ResponseWriter, r *http.Request) {
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
		Key          string `json:"key"`
		ExpiresInSec int    `json:"expires_in_sec"`
		MaxDownloads int    `json:"max_downloads"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	if _, err := s.meta.GetObject(sk, req.Key); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "object not found"})
		return
	}
	token, err := randomToken(24)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "token generation failed"})
		return
	}
	id, _ := randomToken(16)
	now := time.Now().UTC()
	rec := metadata.SharedLinkRecord{
		ID: id, Bucket: sk, Key: req.Key, Token: token,
		MaxDownloads: req.MaxDownloads, CreatedBy: info.Username, CreatedAt: now,
	}
	if req.ExpiresInSec > 0 {
		exp := now.Add(time.Duration(req.ExpiresInSec) * time.Second)
		rec.ExpiresAt = &exp
	}
	if err := s.meta.PutSharedLink(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionShareCreated, "share", bucket+"/"+req.Key)
	s.emitEvent("ShareCreated", map[string]any{"bucket": bucket, "key": req.Key, "token": token})
	writeJSON(w, http.StatusCreated, map[string]any{
		"share": rec,
		"url":   s.publicShareURL(r, token),
	})
}

func (s *Server) handleRevokeSharedLink(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	link, err := s.meta.GetSharedLink(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, link.Bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if err := s.meta.DeleteSharedLink(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublicShareInfo(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	link, status, errBody := s.resolveActivePublicShare(token)
	if status != 0 {
		writeJSON(w, status, errBody)
		return
	}
	rec, err := s.svc.HeadObject(r.Context(), link.Bucket, link.Key, "")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "object not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filename":        shareFilename(link.Key),
		"key":             link.Key,
		"size":            rec.Size,
		"content_type":    rec.ContentType,
		"expires_at":      link.ExpiresAt,
		"max_downloads":   link.MaxDownloads,
		"download_count":  link.DownloadCount,
	})
}

func (s *Server) handlePublicShareDownload(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	link, status, errBody := s.resolveActivePublicShare(token)
	if status != 0 {
		writeJSON(w, status, errBody)
		return
	}
	if _, err := s.meta.IncrementSharedLinkDownload(link.ID); err != nil {
		if errors.Is(err, metadata.ErrShareExpired) {
			s.logActivityAs("public", clientIP(r), metadata.ActionShareExpired, "share", link.Key)
			writeJSON(w, http.StatusGone, map[string]any{"error": "link expired"})
			return
		}
		if errors.Is(err, metadata.ErrShareLimitReached) {
			s.logActivityAs("public", clientIP(r), metadata.ActionShareLimitReached, "share", link.Key)
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "download limit reached"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	rc, rec, err := s.svc.GetObject(r.Context(), link.Bucket, link.Key, "")
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "object not found"})
		return
	}
	defer rc.Close()
	s.logActivityAs("public", clientIP(r), metadata.ActionShareDownloaded, "share", link.Key)
	w.Header().Set("Content-Type", rec.ContentType)
	if rec.ContentType == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Length", strconv.FormatInt(rec.Size, 10))
	w.Header().Set("ETag", rec.ETag)
	_, _ = io.Copy(w, rc)
}

func (s *Server) resolveActivePublicShare(token string) (metadata.SharedLinkRecord, int, map[string]any) {
	link, err := s.meta.GetSharedLinkByToken(token)
	if err != nil {
		return metadata.SharedLinkRecord{}, http.StatusNotFound, map[string]any{"error": "share not found"}
	}
	if err := link.Active(time.Now().UTC()); err != nil {
		if errors.Is(err, metadata.ErrShareExpired) {
			return link, http.StatusGone, map[string]any{"error": "link expired"}
		}
		return link, http.StatusForbidden, map[string]any{"error": "download limit reached"}
	}
	return link, 0, nil
}

func shareFilename(key string) string {
	if key == "" {
		return ""
	}
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[i+1:]
	}
	return path.Base(key)
}

func (s *Server) publicShareURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	}
	host := r.Host
	if host == "" {
		host = "localhost:9000"
	}
	return scheme + "://" + host + "/share/" + token
}

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Server) handleListTenantMembers(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !auth.CanManageTenant(info.Role, s.userTenantMemberships(info.UserID), tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.GetTenant(tenantID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "tenant not found"})
		return
	}
	members, err := s.meta.ListTenantMembers(tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type memberView struct {
		UserID   string              `json:"user_id"`
		Username string              `json:"username"`
		Email    string              `json:"email"`
		Role     string              `json:"role"`
		Groups   []map[string]string `json:"groups"`
	}
	out := make([]memberView, 0, len(members))
	for _, m := range members {
		v := memberView{UserID: m.UserID, Role: m.Role, Groups: s.memberGroupViews(tenantID, m.UserID)}
		if u, err := s.meta.GetUser(m.UserID); err == nil {
			v.Username = u.Username
			v.Email = u.Email
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": out})
}

func (s *Server) handleAddTenantMember(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !auth.CanManageTenant(info.Role, s.userTenantMemberships(info.UserID), tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.GetTenant(tenantID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "tenant not found"})
		return
	}
	var req struct {
		UserID   string   `json:"user_id"`
		Role     string   `json:"role"`
		GroupIDs []string `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "user_id required"})
		return
	}
	if _, err := s.meta.GetUser(req.UserID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	role := req.Role
	if role == "" {
		role = metadata.TenantRoleMember
	}
	if role != metadata.TenantRoleAdmin && role != metadata.TenantRoleMember && role != metadata.TenantRoleViewer {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid role"})
		return
	}
	if !auth.CanAssignTenantRole(info.Role, role) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden role"})
		return
	}
	rec := metadata.TenantMemberRecord{TenantID: tenantID, UserID: req.UserID, Role: role}
	if err := s.meta.PutTenantMember(rec); err != nil {
		if errors.Is(err, metadata.ErrTenantMemberExists) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": "member already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "tenant_member", tenantID)
	if err := s.assignMemberGroups(tenantID, req.UserID, req.GroupIDs); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"member": rec})
}

func (s *Server) handleUpdateTenantMember(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	userID := r.PathValue("userId")
	info, _ := authFrom(r)
	if !auth.CanManageTenant(info.Role, s.userTenantMemberships(info.UserID), tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "role required"})
		return
	}
	if req.Role != metadata.TenantRoleAdmin && req.Role != metadata.TenantRoleMember && req.Role != metadata.TenantRoleViewer {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid role"})
		return
	}
	if !auth.CanAssignTenantRole(info.Role, req.Role) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden role"})
		return
	}
	if err := s.meta.UpdateTenantMemberRole(tenantID, userID, req.Role); err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleCreateTenantUser(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !auth.CanManageTenant(info.Role, s.userTenantMemberships(info.UserID), tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.GetTenant(tenantID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "tenant not found"})
		return
	}
	var req struct {
		Username string   `json:"username"`
		Email    string   `json:"email"`
		Password string   `json:"password"`
		Role     string   `json:"role"`
		GroupIDs []string `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
		return
	}
	tenantRole := req.Role
	if tenantRole == "" {
		tenantRole = metadata.TenantRoleMember
	}
	if !auth.CanAssignTenantRole(info.Role, tenantRole) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid role"})
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hash failed"})
		return
	}
	id := randomID()
	rec := metadata.UserRecord{
		ID: id, Username: req.Username, Email: req.Email,
		PasswordHash: hash, Role: metadata.RoleUser, Status: metadata.StatusActive,
		TenantID: tenantID, CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutUser(rec); err != nil {
		if err == metadata.ErrUserExists {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	memberRec := metadata.TenantMemberRecord{TenantID: tenantID, UserID: id, Role: tenantRole}
	if err := s.meta.PutTenantMember(memberRec); err != nil {
		_ = s.meta.DeleteUser(id)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := s.assignMemberGroups(tenantID, id, req.GroupIDs); err != nil {
		_ = s.meta.DeleteTenantMember(tenantID, id)
		_ = s.meta.DeleteUser(id)
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionUserCreated, "user", req.Username)
	s.emitEvent(metadata.EventUserCreated, map[string]any{"username": req.Username, "user_id": id, "role": metadata.RoleUser, "tenant_id": tenantID})
	writeJSON(w, http.StatusCreated, map[string]any{
		"user": map[string]any{
			"id": id, "user_id": id, "username": req.Username,
			"email": req.Email, "role": metadata.RoleUser, "status": metadata.StatusActive,
		},
		"member": memberRec,
	})
}

func (s *Server) handleRemoveTenantMember(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	userID := r.PathValue("userId")
	info, _ := authFrom(r)
	if !auth.CanManageTenant(info.Role, s.userTenantMemberships(info.UserID), tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if err := s.meta.DeleteTenantMember(tenantID, userID); err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = s.meta.RemoveUserFromTenantGroups(tenantID, userID)
	w.WriteHeader(http.StatusNoContent)
}
