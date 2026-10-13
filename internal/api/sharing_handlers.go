package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) effectiveSharingForRecord(rec metadata.BucketRecord) metadata.EffectiveSharing {
	global, _ := s.meta.GetSharingPolicy(metadata.SharingScopeGlobal, "")
	tenant, _ := s.meta.GetSharingPolicy(metadata.SharingScopeTenant, rec.EffectiveTenantID())
	bucket, _ := s.meta.GetSharingPolicy(metadata.SharingScopeBucket, rec.EffectiveStorageKey())
	return metadata.EffectiveSharingFrom(global, tenant, bucket)
}

func (s *Server) loadSharing(scope, scopeID string) metadata.SharingPolicy {
	rec, err := s.meta.GetSharingPolicy(scope, scopeID)
	if err != nil {
		return metadata.SharingPolicy{Scope: scope, ScopeID: scopeID}
	}
	return rec
}

func (s *Server) parentSharing(scope, tenantID string, bucket metadata.BucketRecord) (metadata.EffectiveSharing, bool, bool) {
	eff := metadata.DefaultEffectiveSharing()
	var shareExplicit, presignExplicit bool
	global := s.loadSharing(metadata.SharingScopeGlobal, "")
	eff, shareExplicit, presignExplicit = metadata.MergeSharing(eff, global, shareExplicit, presignExplicit)
	if scope == metadata.SharingScopeGlobal {
		return metadata.DefaultEffectiveSharing(), false, false
	}
	if scope == metadata.SharingScopeBucket {
		tenant := s.loadSharing(metadata.SharingScopeTenant, bucket.EffectiveTenantID())
		eff, shareExplicit, presignExplicit = metadata.MergeSharing(eff, tenant, shareExplicit, presignExplicit)
	}
	if scope == metadata.SharingScopeTenant && tenantID != "" {
		_ = tenantID
	}
	return eff, shareExplicit, presignExplicit
}

type sharingView struct {
	Policy    metadata.SharingPolicy    `json:"policy"`
	Effective metadata.EffectiveSharing `json:"effective"`
	Locked    []string                  `json:"locked"`
}

func lockedFields(parent metadata.EffectiveSharing, shareExplicit, presignExplicit bool) []string {
	var out []string
	if parent.PublicAccessBlocked {
		out = append(out, "public_access_blocked")
	}
	if !parent.ShareLinksAllowed {
		out = append(out, "share_links_allowed")
	}
	if !parent.PresignAllowed {
		out = append(out, "presign_allowed")
	}
	if parent.SharePasswordRequired {
		out = append(out, "share_password_required")
	}
	_ = shareExplicit
	_ = presignExplicit
	if out == nil {
		out = []string{}
	}
	return out
}

func (s *Server) handleGetGlobalSharing(w http.ResponseWriter, r *http.Request) {
	policy := s.loadSharing(metadata.SharingScopeGlobal, "")
	parent, shareEx, presignEx := s.parentSharing(metadata.SharingScopeGlobal, "", metadata.BucketRecord{})
	eff, _, _ := metadata.MergeSharing(parent, policy, shareEx, presignEx)
	writeJSON(w, http.StatusOK, sharingView{Policy: policy, Effective: eff, Locked: lockedFields(parent, shareEx, presignEx)})
}

func (s *Server) handlePutGlobalSharing(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	s.putSharing(w, r, metadata.SharingScopeGlobal, "", metadata.BucketRecord{}, true, info.Username)
}

func (s *Server) handleGetTenantSharing(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !s.canReadTenantSharing(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.GetTenant(tenantID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "tenant not found"})
		return
	}
	policy := s.loadSharing(metadata.SharingScopeTenant, tenantID)
	parent, shareEx, presignEx := s.parentSharing(metadata.SharingScopeTenant, tenantID, metadata.BucketRecord{})
	eff, _, _ := metadata.MergeSharing(parent, policy, shareEx, presignEx)
	writeJSON(w, http.StatusOK, sharingView{Policy: policy, Effective: eff, Locked: lockedFields(parent, shareEx, presignEx)})
}

func (s *Server) handlePutTenantSharing(w http.ResponseWriter, r *http.Request) {
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
	s.putSharing(w, r, metadata.SharingScopeTenant, tenantID, metadata.BucketRecord{}, false, info.Username)
}

func (s *Server) handleGetBucketSharing(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil || !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	policy := s.loadSharing(metadata.SharingScopeBucket, rec.EffectiveStorageKey())
	parent, shareEx, presignEx := s.parentSharing(metadata.SharingScopeBucket, rec.EffectiveTenantID(), rec)
	eff := s.effectiveSharingForRecord(rec)
	writeJSON(w, http.StatusOK, sharingView{
		Policy: policy, Effective: eff, Locked: lockedFields(parent, shareEx, presignEx),
	})
}

func (s *Server) handlePutBucketSharing(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !s.isBucketAdmin(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	s.putSharing(w, r, metadata.SharingScopeBucket, rec.EffectiveStorageKey(), rec, false, info.Username)
}

func (s *Server) putSharing(w http.ResponseWriter, r *http.Request, scope, scopeID string, bucket metadata.BucketRecord, topLevel bool, actor string) {
	var proposed metadata.SharingPolicy
	if err := json.NewDecoder(r.Body).Decode(&proposed); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	parent, shareEx, presignEx := s.parentSharing(scope, scopeID, bucket)
	if why := metadata.SharingLoosens(parent, shareEx, presignEx, topLevel, proposed); why != "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "sharing_loosens", "field": why})
		return
	}
	proposed.Scope = scope
	proposed.ScopeID = scopeID
	proposed.UpdatedAt = time.Now().UTC()
	proposed.UpdatedBy = actor
	if err := s.meta.PutSharingPolicy(proposed); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSharingPolicyChanged, "sharing", scope+":"+scopeID)
	s.publishEvent(r.Context(), "SharingPolicyChanged", bucket.Name, "", "", bucket.EffectiveTenantID(), map[string]any{
		"scope": scope, "scope_id": scopeID, "actor": actor, "client_ip": clientIP(r),
	})
	eff, _, _ := metadata.MergeSharing(parent, proposed, shareEx, presignEx)
	if scope == metadata.SharingScopeBucket {
		eff = s.effectiveSharingForRecord(bucket)
	}
	writeJSON(w, http.StatusOK, sharingView{Policy: proposed, Effective: eff, Locked: lockedFields(parent, shareEx, presignEx)})
}

func (s *Server) canReadTenantSharing(info auth.TokenInfo, tenantID string) bool {
	if auth.IsAdmin(info.Role) || auth.CanSeeAllBuckets(info.Role) {
		return true
	}
	for _, m := range s.userTenantMemberships(info.UserID) {
		if m.TenantID == tenantID {
			return true
		}
	}
	return false
}

func (s *Server) handleRevokeAllSharedLinks(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	rec, err := s.resolveBucketForUser(info, bucket)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if !s.isBucketAdmin(info, rec) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	links, err := s.linksForBucket(rec)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	n := 0
	for _, link := range links {
		if err := s.meta.DeleteSharedLink(link.ID); err != nil {
			continue
		}
		n++
		s.publishShareEvent(r, "ShareRevoked", rec, link, info.Username)
	}
	s.logActivity(r, metadata.ActionShareRevoked, "share", bucket)
	writeJSON(w, http.StatusOK, map[string]any{"revoked": n})
}

func (s *Server) linksForBucket(rec metadata.BucketRecord) ([]metadata.SharedLinkRecord, error) {
	sk := rec.EffectiveStorageKey()
	links, err := s.meta.ListSharedLinks(sk, "")
	if err != nil {
		return nil, err
	}
	if rec.Name != "" && rec.Name != sk {
		more, err := s.meta.ListSharedLinks(rec.Name, "")
		if err != nil {
			return nil, err
		}
		links = append(links, more...)
	}
	return links, nil
}

func (s *Server) publishEvent(ctx context.Context, eventType, bucket, key, versionID, tenantID string, payload map[string]any) {
	evs := s.outboxRecords(eventType, bucket, key, versionID, tenantID, payload)
	if len(evs) == 0 {
		return
	}
	ins, ok := s.meta.(interface {
		InsertOutbox(context.Context, events.OutboxRecord) (int, error)
	})
	if !ok {
		return
	}
	for _, ev := range evs {
		dropped, err := ins.InsertOutbox(ctx, ev)
		if err == nil {
			s.noteOutboxDropped(dropped)
		}
	}
}

func (s *Server) publishShareEvent(r *http.Request, eventType string, rec metadata.BucketRecord, link metadata.SharedLinkRecord, actor string) {
	payload := map[string]any{
		"link_id":    link.ID,
		"actor":      actor,
		"client_ip":  clientIP(r),
		"bucket":     rec.Name,
		"key":        link.Key,
		"user_agent": r.UserAgent(),
	}
	s.publishEvent(r.Context(), eventType, rec.Name, link.Key, "", rec.EffectiveTenantID(), payload)
}

func (s *Server) bucketByStoredName(name string) (metadata.BucketRecord, error) {
	if rec, err := s.meta.GetBucketByKey(name); err == nil {
		return rec, nil
	}
	return s.meta.GetBucket(name)
}

func (s *Server) mayCreatePublicRead(info auth.TokenInfo) bool {
	if auth.IsAdmin(info.Role) {
		return true
	}
	return s.isTenantAdmin(info, s.primaryTenantIDForUser(info.UserID))
}

func (s *Server) publicAccessBlockedForNewBucket(info auth.TokenInfo) bool {
	global := s.loadSharing(metadata.SharingScopeGlobal, "")
	tenant := s.loadSharing(metadata.SharingScopeTenant, s.primaryTenantIDForUser(info.UserID))
	eff := metadata.EffectiveSharingFrom(global, tenant, metadata.SharingPolicy{})
	return eff.PublicAccessBlocked
}

func sharingDisabledBody() map[string]any {
	return map[string]any{
		"error":      "sharing_disabled",
		"message":    s3.ShareDisabledMessage,
		"message_ru": s3.ShareDisabledMessageRU,
	}
}
