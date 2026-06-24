package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) tenantGroupAccessForUser(userID, bucketKey string) *auth.GroupBucketAccess {
	accesses, _ := s.meta.ListUserGroupBucketAccess(userID)
	for _, a := range accesses {
		if a.BucketKey == bucketKey {
			return &auth.GroupBucketAccess{CanRead: a.CanRead, CanWrite: a.CanWrite}
		}
	}
	return nil
}

func (s *Server) tenantHasGroups(tenantID string) bool {
	n, _ := s.meta.CountTenantGroups(tenantID)
	return n > 0
}

func (s *Server) bucketInTenantGroup(tenantID, bucketKey string) bool {
	keys, _ := s.meta.ListTenantGroupBucketKeys(tenantID)
	for _, k := range keys {
		if k == bucketKey {
			return true
		}
	}
	return false
}

func (s *Server) grantBucketKeysForUser(userID string) map[string]struct{} {
	out := map[string]struct{}{}
	buckets, _ := s.meta.ListBuckets()
	for _, b := range buckets {
		grants, _ := s.meta.ListBucketAccessGrants(b.EffectiveStorageKey())
		for _, g := range grants {
			if g.UserID == userID && (g.CanRead || g.CanWrite) {
				out[b.EffectiveStorageKey()] = struct{}{}
			}
		}
		prefixGrants, _ := s.meta.ListBucketPrefixAccessGrants(b.EffectiveStorageKey())
		for _, g := range prefixGrants {
			if g.UserID == userID && (g.CanRead || g.CanWrite) {
				out[b.EffectiveStorageKey()] = struct{}{}
			}
		}
	}
	return out
}

func (s *Server) groupBucketKeysForUser(userID string) map[string]struct{} {
	out := map[string]struct{}{}
	accesses, _ := s.meta.ListUserGroupBucketAccess(userID)
	for _, a := range accesses {
		out[a.BucketKey] = struct{}{}
	}
	return out
}

func (s *Server) tenantsWithGroupsForUser(memberships []auth.TenantMembership) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range memberships {
		if s.tenantHasGroups(m.TenantID) {
			out[m.TenantID] = struct{}{}
		}
	}
	return out
}

func (s *Server) tenantAdminIDs(memberships []auth.TenantMembership) []string {
	var out []string
	for _, m := range memberships {
		if m.Role == auth.TenantRoleAdmin {
			out = append(out, m.TenantID)
		}
	}
	return out
}

func (s *Server) prefixGrantsForRBAC(grants []metadata.BucketPrefixAccessGrant) []auth.PrefixGrant {
	out := make([]auth.PrefixGrant, 0, len(grants))
	for _, g := range grants {
		out = append(out, auth.PrefixGrant{UserID: g.UserID, Prefix: g.Prefix, CanRead: g.CanRead, CanWrite: g.CanWrite})
	}
	return out
}

func (s *Server) bucketPrefixGrantsFor(rec metadata.BucketRecord) []metadata.BucketPrefixAccessGrant {
	grants, _ := s.meta.ListBucketPrefixAccessGrants(rec.EffectiveStorageKey())
	return grants
}

func (s *Server) bucketUsesPrefixGrants(rec metadata.BucketRecord) bool {
	n, _ := s.meta.CountBucketPrefixAccessGrants(rec.EffectiveStorageKey())
	return n > 0
}

func (s *Server) tenantBucketAccessInput(info auth.TokenInfo, rec metadata.BucketRecord) auth.TenantBucketAccessInput {
	grants := s.bucketGrantsFor(rec)
	prefixGrants := s.bucketPrefixGrantsFor(rec)
	bucketKey := rec.EffectiveStorageKey()
	return auth.TenantBucketAccessInput{
		Role:                info.Role,
		UserID:              info.UserID,
		Username:            info.Username,
		TeamIDs:             s.userTeamIDs(info.UserID),
		BucketOwnerID:       rec.OwnerID,
		BucketOwner:         rec.Owner,
		BucketTeamID:        rec.TeamID,
		BucketTenantID:      rec.TenantID,
		BucketKey:           bucketKey,
		UserTenants:         s.userTenantMemberships(info.UserID),
		Grants:              s.grantsForRBAC(grants),
		PrefixGrants:        s.prefixGrantsForRBAC(prefixGrants),
		HasGrants:           s.bucketUsesGrants(rec),
		HasPrefixGrants:     s.bucketUsesPrefixGrants(rec),
		GroupAccess:         s.tenantGroupAccessForUser(info.UserID, bucketKey),
		TenantHasGroups:     s.tenantHasGroups(rec.TenantID),
		BucketInTenantGroup: s.bucketInTenantGroup(rec.TenantID, bucketKey),
	}
}

func (s *Server) resolveBucketAccess(info auth.TokenInfo, rec metadata.BucketRecord, write bool) bool {
	in := s.tenantBucketAccessInput(info, rec)
	if in.BucketTenantID != "" || in.HasGrants || in.HasPrefixGrants || in.TenantHasGroups {
		return auth.ResolveTenantBucketAccess(in, write)
	}
	if write {
		return auth.CanWriteBucket(in.Role, in.UserID, in.Username, in.TeamIDs,
			in.BucketOwnerID, in.BucketOwner, in.BucketTeamID, in.BucketTenantID, in.UserTenants)
	}
	return auth.CanAccessBucket(in.Role, in.UserID, in.Username, in.TeamIDs,
		in.BucketOwnerID, in.BucketOwner, in.BucketTeamID, in.BucketTenantID, in.UserTenants)
}

func (s *Server) handleListTenantGroups(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	groups, err := s.meta.ListTenantGroups(tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type groupView struct {
		ID          string `json:"id"`
		TenantID    string `json:"tenant_id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		AccessLevel string `json:"access_level"`
		CreatedAt   string `json:"created_at"`
		BucketCount int    `json:"bucket_count"`
		MemberCount int    `json:"member_count"`
	}
	out := make([]groupView, 0, len(groups))
	for _, g := range groups {
		buckets, _ := s.meta.ListTenantGroupBuckets(g.ID)
		members, _ := s.meta.ListTenantGroupMembers(g.ID)
		out = append(out, groupView{
			ID: g.ID, TenantID: g.TenantID, Name: g.Name, Description: g.Description,
			AccessLevel: g.AccessLevel, CreatedAt: g.CreatedAt.Format(time.RFC3339),
			BucketCount: len(buckets), MemberCount: len(members),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": out})
}

func (s *Server) handleCreateTenantGroup(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.GetTenant(tenantID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "tenant not found"})
		return
	}
	var req struct {
		Name         string `json:"name"`
		ExternalName string `json:"external_name"`
		Description  string `json:"description"`
		AccessLevel  string `json:"access_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	rec := metadata.TenantGroupRecord{
		ID: randomID(), TenantID: tenantID, Name: req.Name, ExternalName: req.ExternalName,
		Description: req.Description, AccessLevel: req.AccessLevel, CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutTenantGroup(rec); err != nil {
		if errors.Is(err, metadata.ErrTenantGroupExists) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"group": rec})
}

func (s *Server) handleGetTenantGroup(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	groupID := r.PathValue("group_id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.meta.GetTenantGroup(groupID)
	if err != nil || rec.TenantID != tenantID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	buckets, _ := s.meta.ListTenantGroupBuckets(groupID)
	members, _ := s.meta.ListTenantGroupMembers(groupID)
	writeJSON(w, http.StatusOK, map[string]any{
		"group": rec, "bucket_keys": buckets, "member_ids": members,
	})
}

func (s *Server) handleUpdateTenantGroup(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	groupID := r.PathValue("group_id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.meta.GetTenantGroup(groupID)
	if err != nil || rec.TenantID != tenantID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		Name         string  `json:"name"`
		ExternalName *string `json:"external_name"`
		Description  string  `json:"description"`
		AccessLevel  string  `json:"access_level"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name != "" {
		rec.Name = req.Name
	}
	if req.ExternalName != nil {
		rec.ExternalName = *req.ExternalName
	}
	rec.Description = req.Description
	if req.AccessLevel != "" {
		rec.AccessLevel = req.AccessLevel
	}
	if err := s.meta.PutTenantGroup(rec); err != nil {
		if errors.Is(err, metadata.ErrTenantGroupExists) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"group": rec})
}

func (s *Server) handleDeleteTenantGroup(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	groupID := r.PathValue("group_id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.meta.GetTenantGroup(groupID)
	if err != nil || rec.TenantID != tenantID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	if err := s.meta.DeleteTenantGroup(groupID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListTenantBuckets(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	buckets, err := s.meta.ListBucketsByTenant(tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"buckets": buckets, "tenant_id": tenantID})
}

func (s *Server) handlePutTenantGroupBuckets(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	groupID := r.PathValue("group_id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	rec, err := s.meta.GetTenantGroup(groupID)
	if err != nil || rec.TenantID != tenantID {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	var req struct {
		BucketKeys []string `json:"bucket_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	for _, bk := range req.BucketKeys {
		if bk == "" {
			continue
		}
		brec, berr := s.meta.GetBucketByKey(bk)
		if berr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bucket not found: " + bk})
			return
		}
		if !metadata.BucketBelongsToTenant(brec, tenantID, s.tenantMemberOwnerSet(tenantID)) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bucket not in tenant"})
			return
		}
		if brec.TenantID != tenantID {
			brec.TenantID = tenantID
			_ = s.meta.UpdateBucket(brec)
		}
	}
	if err := s.meta.ReplaceTenantGroupBuckets(groupID, req.BucketKeys); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bucket_keys": req.BucketKeys})
}

func (s *Server) handlePutMemberGroups(w http.ResponseWriter, r *http.Request) {
	tenantID := r.PathValue("id")
	userID := r.PathValue("user_id")
	info, _ := authFrom(r)
	if !s.isTenantAdmin(info, tenantID) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	if _, err := s.meta.GetTenantMember(tenantID, userID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "member not found"})
		return
	}
	var req struct {
		GroupIDs []string `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if err := s.meta.ReplaceUserTenantGroups(tenantID, userID, req.GroupIDs); err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid group"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "group_ids": req.GroupIDs})
}

func (s *Server) assignMemberGroups(tenantID, userID string, groupIDs []string) error {
	if len(groupIDs) == 0 {
		return nil
	}
	return s.meta.ReplaceUserTenantGroups(tenantID, userID, groupIDs)
}

func (s *Server) memberGroupViews(tenantID, userID string) []map[string]string {
	ids, _ := s.meta.ListUserTenantGroupIDs(tenantID, userID)
	out := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		if g, err := s.meta.GetTenantGroup(id); err == nil {
			out = append(out, map[string]string{"id": g.ID, "name": g.Name})
		}
	}
	return out
}
