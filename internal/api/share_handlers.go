package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) applyBucketVisibilityPolicy(info auth.TokenInfo, logicalBucket, visibility string) error {
	rec, err := s.resolveBucketForUser(info, logicalBucket)
	if err != nil {
		return err
	}
	if visibility == "public-read" {
		if !s.isBucketAdmin(info, rec) && !auth.IsAdmin(info.Role) {
			return errSharingForbidden
		}
		if s.effectiveSharingForRecord(rec).PublicAccessBlocked {
			return errPublicBlocked
		}
		if rec.DefaultEncryption == "aws:kms" {
			return errPublicKMS
		}
		rec.Policy = publicReadBucketPolicy(logicalBucket)
	} else if visibility == "private" && rec.Policy == publicReadBucketPolicy(logicalBucket) {
		rec.Policy = ""
	}
	rec.Visibility = visibility
	return s.meta.UpdateBucket(rec)
}

var (
	errSharingForbidden = errors.New("forbidden")
	errPublicBlocked    = errors.New("public_access_blocked")
	errPublicKMS        = errors.New("public_kms_conflict")
)

func (s *Server) handleListSharedLinks(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	key := r.URL.Query().Get("key")
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
	links, err := s.linksForBucket(rec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	seeAll := s.isBucketAdmin(info, rec)
	var out []shareAPIView
	for _, link := range links {
		if key != "" && link.Key != key {
			continue
		}
		if !seeAll && link.CreatedBy != info.Username {
			continue
		}
		out = append(out, shareViewOf(link, false))
	}
	if out == nil {
		out = []shareAPIView{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"shares": out})
}

func (s *Server) handleCreateSharedLink(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canWriteBucket(info, bucket) {
		s.noteShareDenied(r, bucket, "forbidden")
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	sk, ok := s.bucketKeyOr404(w, info, bucket)
	if !ok {
		return
	}
	brec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	eff := s.effectiveSharingForRecord(brec)
	if !eff.ShareLinksAllowed {
		s.noteShareDenied(r, bucket, "sharing_disabled")
		writeJSON(w, http.StatusForbidden, sharingDisabledBody())
		return
	}
	var req struct {
		Key          string `json:"key"`
		ExpiresInSec int    `json:"expires_in_sec"`
		MaxDownloads int    `json:"max_downloads"`
		Password     string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "key required"})
		return
	}
	if eff.SharePasswordRequired && strings.TrimSpace(req.Password) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "password_required"})
		return
	}
	expires := req.ExpiresInSec
	if eff.MaxShareTTLSec > 0 {
		if expires <= 0 || int64(expires) > eff.MaxShareTTLSec {
			if expires > 0 && int64(expires) > eff.MaxShareTTLSec {
				writeJSON(w, http.StatusBadRequest, map[string]any{"error": "max_share_ttl", "max_share_ttl_sec": eff.MaxShareTTLSec})
				return
			}
			expires = int(eff.MaxShareTTLSec)
		}
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
	if strings.TrimSpace(req.Password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "password hash failed"})
			return
		}
		rec.PasswordHash = string(hash)
	}
	if expires > 0 {
		exp := now.Add(time.Duration(expires) * time.Second)
		rec.ExpiresAt = &exp
	}
	evs := s.outboxRecords("ShareCreated", bucket, req.Key, "", "", map[string]any{
		"bucket": bucket, "key": req.Key, "link_id": id, "actor": info.Username,
		"client_ip": clientIP(r), "expires_in_sec": expires, "max_downloads": req.MaxDownloads,
		"has_password": rec.PasswordHash != "",
	})
	var dropped int
	if wmut, ok := s.meta.(metadata.OutboxMutator); ok && len(evs) > 0 {
		dropped, err = wmut.PutSharedLinkWithOutbox(rec, evs)
	} else {
		err = s.meta.PutSharedLink(rec)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.noteOutboxDropped(dropped)
	s.logActivity(r, metadata.ActionShareCreated, "share", bucket+"/"+req.Key)
	view := shareViewOf(rec, true)
	view.Token = token
	writeJSON(w, http.StatusCreated, map[string]any{
		"share": view,
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
	brec, err := s.bucketByStoredName(link.Bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if link.CreatedBy != info.Username && !s.isBucketAdmin(info, brec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if err := s.meta.DeleteSharedLink(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionShareRevoked, "share", brec.Name+"/"+link.Key)
	s.publishShareEvent(r, "ShareRevoked", brec, link, info.Username)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePublicShareInfo(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	link, status, errBody := s.resolveActivePublicShare(r, token, false)
	if status != 0 {
		writeJSON(w, status, errBody)
		return
	}
	if link.PasswordHash != "" {
		if sharePasswordFrom(r) == "" {
			writeJSON(w, http.StatusOK, map[string]any{"password_required": true})
			return
		}
		if !s.sharePasswordOK(r, &link) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid_password"})
			return
		}
	}
	rec, err := s.svc.HeadObject(r.Context(), link.Bucket, link.Key, "")
	if err == nil {
		err = s.svc.ProbeSealedObject(rec)
	}
	if err != nil {
		if errors.Is(err, s3.ErrKeyDestroyed) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": s3.MsgKeyDestroyed})
			return
		}
		if errors.Is(err, s3.ErrKMSUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "KMS unavailable",
				"message": "Файл временно недоступен, попробуйте позже",
			})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "object not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"filename":          shareFilename(link.Key),
		"key":               link.Key,
		"size":              rec.Size,
		"content_type":      rec.ContentType,
		"expires_at":        link.ExpiresAt,
		"max_downloads":     link.MaxDownloads,
		"download_count":    link.DownloadCount,
		"password_required": link.PasswordHash != "",
	})
}

func (s *Server) handlePublicShareDownload(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	link, status, errBody := s.resolveActivePublicShare(r, token, true)
	if status != 0 {
		writeJSON(w, status, errBody)
		return
	}
	brec, _ := s.bucketByStoredName(link.Bucket)
	if obj, oerr := s.meta.GetObject(link.Bucket, link.Key); oerr == nil && obj.Encryption.Sealed() {
		if !s.shareCreatorCanRead(link, brec) {
			writeJSON(w, http.StatusForbidden, map[string]any{"error": "signer is not allowed to read this encrypted object"})
			return
		}
	}
	if _, err := s.meta.IncrementSharedLinkDownload(link.ID); err != nil {
		if errors.Is(err, metadata.ErrShareExpired) || errors.Is(err, metadata.ErrShareLimitReached) {
			s.noteShareTerminal(r, link, err)
			status := http.StatusForbidden
			msg := "download limit reached"
			if errors.Is(err, metadata.ErrShareExpired) {
				status = http.StatusGone
				msg = "link expired"
			}
			writeJSON(w, status, map[string]any{"error": msg})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	rc, rec, err := s.svc.GetObject(r.Context(), link.Bucket, link.Key, "")
	if err != nil {
		if errors.Is(err, s3.ErrKeyDestroyed) {
			resource := link.ID + " result=key_destroyed"
			payload := map[string]any{
				"link_id": link.ID, "actor": "anonymous", "client_ip": clientIP(r),
				"user_agent": r.UserAgent(), "bucket": brec.Name, "key": link.Key,
				"result": "key_destroyed",
			}
			if rec.Encryption != nil && rec.Encryption.KeyID != "" {
				resource = link.ID + " key_id=" + rec.Encryption.KeyID + " key_version=" + strconv.Itoa(rec.Encryption.KeyVersion) + " result=key_destroyed"
				payload["key_id"] = rec.Encryption.KeyID
				payload["key_version"] = rec.Encryption.KeyVersion
			}
			s.logActivityAs("public", clientIP(r), metadata.ActionShareDownloaded, "share", resource)
			s.publishEvent(r.Context(), "ShareDownloaded", brec.Name, link.Key, "", brec.EffectiveTenantID(), payload)
			writeJSON(w, http.StatusForbidden, map[string]any{"error": s3.MsgKeyDestroyed})
			return
		}
		if errors.Is(err, s3.ErrKMSUnavailable) {
			resource := link.ID + " result=kms_unavailable"
			payload := map[string]any{
				"link_id": link.ID, "actor": "anonymous", "client_ip": clientIP(r),
				"user_agent": r.UserAgent(), "bucket": brec.Name, "key": link.Key,
				"result": "kms_unavailable",
			}
			if rec.Encryption != nil && rec.Encryption.KeyID != "" {
				resource = link.ID + " key_id=" + rec.Encryption.KeyID + " key_version=" + strconv.Itoa(rec.Encryption.KeyVersion) + " result=kms_unavailable"
				payload["key_id"] = rec.Encryption.KeyID
				payload["key_version"] = rec.Encryption.KeyVersion
			}
			s.logActivityAs("public", clientIP(r), metadata.ActionShareDownloaded, "share", resource)
			s.publishEvent(r.Context(), "ShareDownloaded", brec.Name, link.Key, "", brec.EffectiveTenantID(), payload)
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"error":   "KMS unavailable",
				"message": "Файл временно недоступен, попробуйте позже",
			})
			return
		}
		if strings.Contains(err.Error(), "message authentication failed") {
			w.Header().Set("Connection", "close")
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "object not found"})
		return
	}
	defer rc.Close()
	shareResource := link.ID
	if rec.Encryption != nil && rec.Encryption.Sealed() {
		shareResource = link.ID + " key_id=" + rec.Encryption.KeyID + " key_version=" + strconv.Itoa(rec.Encryption.KeyVersion)
	}
	s.logActivityAs("public", clientIP(r), metadata.ActionShareDownloaded, "share", shareResource)
	payload := map[string]any{
		"link_id": link.ID, "actor": "anonymous", "client_ip": clientIP(r),
		"user_agent": r.UserAgent(), "size": rec.Size, "bucket": brec.Name, "key": link.Key,
	}
	if rec.Encryption != nil && rec.Encryption.Sealed() {
		payload["key_id"] = rec.Encryption.KeyID
		payload["key_version"] = rec.Encryption.KeyVersion
	}
	s.publishEvent(r.Context(), "ShareDownloaded", brec.Name, link.Key, "", brec.EffectiveTenantID(), payload)
	w.Header().Set("Content-Type", rec.ContentType)
	if rec.ContentType == "" {
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Header().Set("Content-Length", strconv.FormatInt(rec.Size, 10))
	w.Header().Set("ETag", rec.ETag)
	_, _ = io.Copy(w, rc)
}

func (s *Server) resolveActivePublicShare(r *http.Request, token string, requirePassword bool) (metadata.SharedLinkRecord, int, map[string]any) {
	link, err := s.meta.GetSharedLinkByToken(token)
	if err != nil {
		return metadata.SharedLinkRecord{}, http.StatusNotFound, map[string]any{"error": "share not found"}
	}
	if brec, berr := s.bucketByStoredName(link.Bucket); berr == nil {
		if !s.effectiveSharingForRecord(brec).ShareLinksAllowed {
			s.logActivityAs("public", clientIP(r), metadata.ActionShareBlocked, "share", link.ID)
			s.publishShareEvent(r, "ShareBlocked", brec, link, "anonymous")
			return link, http.StatusForbidden, sharingDisabledBody()
		}
	}
	if err := link.Active(time.Now().UTC()); err != nil {
		// Active fails before IncrementSharedLinkDownload, so the event has
		// to be claimed here. The first rejection emits once; later ones do not.
		s.noteShareTerminal(r, link, err)
		if errors.Is(err, metadata.ErrShareExpired) {
			return link, http.StatusGone, map[string]any{"error": "link expired"}
		}
		return link, http.StatusForbidden, map[string]any{"error": "download limit reached"}
	}
	if requirePassword && link.PasswordHash != "" && !s.sharePasswordOK(r, &link) {
		return link, http.StatusUnauthorized, map[string]any{"error": "password_required"}
	}
	return link, 0, nil
}

// noteShareTerminal emits ShareExpired or ShareLimitReached and the matching
// Activity row once per link. A repeated reject does not write another row.
func (s *Server) noteShareTerminal(r *http.Request, link metadata.SharedLinkRecord, cause error) {
	kind, action, eventType := "", "", ""
	switch {
	case errors.Is(cause, metadata.ErrShareExpired):
		kind, action, eventType = "expired", metadata.ActionShareExpired, "ShareExpired"
	case errors.Is(cause, metadata.ErrShareLimitReached):
		kind, action, eventType = "limit", metadata.ActionShareLimitReached, "ShareLimitReached"
	default:
		return
	}
	claimed, err := s.meta.ClaimShareTerminalEvent(link.ID, kind, time.Now().UTC())
	if err != nil || !claimed {
		return
	}
	brec, _ := s.bucketByStoredName(link.Bucket)
	s.logActivityAs("public", clientIP(r), action, "share", link.ID)
	s.publishShareEvent(r, eventType, brec, link, "anonymous")
}

func (s *Server) shareCreatorCanRead(link metadata.SharedLinkRecord, brec metadata.BucketRecord) bool {
	if link.CreatedBy == "" {
		return false
	}
	user, err := s.meta.GetUserByUsername(link.CreatedBy)
	if err != nil || user.Status != metadata.StatusActive {
		return false
	}
	info := auth.TokenInfo{UserID: user.ID, Username: user.Username, Role: user.Role}
	name := brec.Name
	if name == "" {
		name = link.Bucket
	}
	return s.canAccessBucket(info, name)
}

func (s *Server) sharePasswordOK(r *http.Request, link *metadata.SharedLinkRecord) bool {
	if link.PasswordHash == "" {
		return true
	}
	now := time.Now().UTC()
	if link.PasswordFailures >= 5 && link.PasswordFailureAt != nil && now.Sub(*link.PasswordFailureAt) < 5*time.Minute {
		return false
	}
	password := sharePasswordFrom(r)
	if password == "" {
		return false
	}
	if bcrypt.CompareHashAndPassword([]byte(link.PasswordHash), []byte(password)) != nil {
		failures := link.PasswordFailures + 1
		if link.PasswordFailureAt == nil || now.Sub(*link.PasswordFailureAt) >= 5*time.Minute {
			failures = 1
		}
		at := now
		_ = s.meta.TouchSharePasswordFailure(link.ID, failures, &at)
		s.logActivityAs("public", clientIP(r), "share.password_failed", "share", link.ID)
		return false
	}
	if link.PasswordFailures > 0 {
		_ = s.meta.TouchSharePasswordFailure(link.ID, 0, nil)
	}
	return true
}

type shareAPIView struct {
	ID            string     `json:"id"`
	Bucket        string     `json:"bucket"`
	Key           string     `json:"key"`
	Token         string     `json:"token,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	MaxDownloads  int        `json:"max_downloads"`
	DownloadCount int        `json:"download_count"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	HasPassword   bool       `json:"has_password"`
}

func shareViewOf(rec metadata.SharedLinkRecord, withToken bool) shareAPIView {
	view := shareAPIView{
		ID: rec.ID, Bucket: rec.Bucket, Key: rec.Key,
		ExpiresAt: rec.ExpiresAt, MaxDownloads: rec.MaxDownloads, DownloadCount: rec.DownloadCount,
		CreatedBy: rec.CreatedBy, CreatedAt: rec.CreatedAt, HasPassword: rec.PasswordHash != "",
	}
	if withToken {
		view.Token = rec.Token
	}
	return view
}

func sharePasswordFrom(r *http.Request) string {
	if p := r.Header.Get("X-DataSafe-Share-Password"); p != "" {
		return p
	}
	return r.URL.Query().Get("password")
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

func firstForwardedValue(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.Index(v, ","); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

func (s *Server) publicShareURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if fwd := firstForwardedValue(r.Header.Get("X-Forwarded-Proto")); fwd != "" {
		scheme = fwd
	}
	host := firstForwardedValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
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
	if role == metadata.TenantRoleAdmin {
		s.noteTenantAdmin(req.UserID)
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
	if req.Role == metadata.TenantRoleAdmin {
		s.noteTenantAdmin(userID)
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
	if evs := s.outboxRecords(metadata.EventUserCreated, "", "", "", tenantID, map[string]any{"username": req.Username, "user_id": id, "role": metadata.RoleUser, "tenant_id": tenantID}); len(evs) > 0 {
		if st, ok := s.meta.(events.OutboxStore); ok {
			if dropped, insErr := st.InsertOutbox(r.Context(), evs[0]); insErr != nil {
				slog.Error("event outbox insert failed after tenant user create", "err", insErr)
			} else {
				s.noteOutboxDropped(dropped)
			}
		}
	}
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
