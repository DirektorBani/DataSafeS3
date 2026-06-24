package api

import (
	"net/http"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) storageKeyForLogicalBucket(logical string) string {
	if rec, err := s.meta.GetBucket(logical); err == nil {
		return rec.EffectiveStorageKey()
	}
	return logical
}

func (s *Server) userTeamIDs(userID string) []string {
	extra, _ := s.meta.ListUserTeamIDs(userID)
	if user, err := s.meta.GetUser(userID); err == nil {
		return metadata.MergeTeamIDs(user.TeamID, extra)
	}
	return extra
}

func (s *Server) userTenantMemberships(userID string) []auth.TenantMembership {
	recs, _ := s.meta.ListUserTenants(userID)
	out := make([]auth.TenantMembership, 0, len(recs))
	for _, r := range recs {
		out = append(out, auth.TenantMembership{TenantID: r.TenantID, Role: r.Role})
	}
	return out
}

func tenantIDsFromMemberships(m []auth.TenantMembership) []string {
	out := make([]string, 0, len(m))
	for _, t := range m {
		if t.TenantID != "" {
			out = append(out, t.TenantID)
		}
	}
	return out
}

func (s *Server) bucketScopeForUser(info auth.TokenInfo) metadata.BucketScope {
	userTenant := ""
	if user, err := s.meta.GetUser(info.UserID); err == nil {
		userTenant = user.TenantID
	}
	members, _ := s.meta.ListUserTenants(info.UserID)
	return metadata.BucketScopeForUser(userTenant, info.UserID, members)
}

func (s *Server) resolveBucketForUser(info auth.TokenInfo, logicalName string) (metadata.BucketRecord, error) {
	if auth.CanSeeAllBuckets(info.Role) {
		if rec, err := s.meta.GetBucketByKey(logicalName); err == nil {
			return rec, nil
		}
	}
	scope := s.bucketScopeForUser(info)
	if rec, err := s.meta.ResolveBucket(scope, logicalName); err == nil {
		return rec, nil
	}
	// Tenant members may still own personal buckets in the default namespace.
	if scope.Kind != metadata.ScopeOwner && info.UserID != "" {
		ownerScope := metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: info.UserID}
		if rec, err := s.meta.ResolveBucket(ownerScope, logicalName); err == nil {
			return rec, nil
		}
	}
	return s.meta.GetBucket(logicalName)
}

func (s *Server) bucketStorageKey(info auth.TokenInfo, logicalName string) (string, metadata.BucketRecord, error) {
	rec, err := s.resolveBucketForUser(info, logicalName)
	if err != nil {
		return "", rec, err
	}
	return rec.EffectiveStorageKey(), rec, nil
}

func (s *Server) bucketKeyOr404(w http.ResponseWriter, info auth.TokenInfo, logicalName string) (string, bool) {
	sk, _, err := s.bucketStorageKey(info, logicalName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return "", false
	}
	return sk, true
}

func (s *Server) grantsForRBAC(grants []metadata.BucketAccessGrant) []auth.BucketGrant {
	out := make([]auth.BucketGrant, 0, len(grants))
	for _, g := range grants {
		out = append(out, auth.BucketGrant{UserID: g.UserID, CanRead: g.CanRead, CanWrite: g.CanWrite})
	}
	return out
}

func (s *Server) bucketGrantsFor(rec metadata.BucketRecord) []metadata.BucketAccessGrant {
	grants, _ := s.meta.ListBucketAccessGrants(rec.EffectiveStorageKey())
	return grants
}

func (s *Server) bucketUsesGrants(rec metadata.BucketRecord) bool {
	n, _ := s.meta.CountBucketAccessGrants(rec.EffectiveStorageKey())
	return n > 0
}

// usageIncludesTransferStats reports whether upload/download counters are shown.
func (s *Server) usageIncludesTransferStats(info auth.TokenInfo, buckets []metadata.BucketRecord) bool {
	if auth.CanSeeSystemUsage(info.Role) {
		return true
	}
	for _, m := range s.userTenantMemberships(info.UserID) {
		if auth.TenantRoleCanWrite(m.Role) {
			return true
		}
	}
	for _, b := range buckets {
		if b.OwnerID == info.UserID || b.Owner == info.Username {
			return true
		}
	}
	return false
}

func (s *Server) bucketListFilter(info auth.TokenInfo) metadata.BucketListFilter {
	if auth.CanSeeAllBuckets(info.Role) {
		return metadata.BucketListFilter{Unfiltered: true}
	}
	tenants := s.userTenantMemberships(info.UserID)
	return metadata.BucketListFilter{
		UserID:            info.UserID,
		Username:          info.Username,
		TeamIDs:           s.userTeamIDs(info.UserID),
		TenantIDs:         tenantIDsFromMemberships(tenants),
		TenantAdminIDs:    s.tenantAdminIDs(tenants),
		GroupBucketKeys:   s.groupBucketKeysForUser(info.UserID),
		TenantsWithGroups: s.tenantsWithGroupsForUser(tenants),
		GrantBucketKeys:   s.grantBucketKeysForUser(info.UserID),
	}
}

func (s *Server) canAccessBucket(info auth.TokenInfo, bucketName string) bool {
	if auth.CanSeeAllBuckets(info.Role) {
		return true
	}
	rec, err := s.resolveBucketForUser(info, bucketName)
	if err != nil {
		return false
	}
	return s.resolveBucketAccess(info, rec, false)
}

func (s *Server) canWriteBucket(info auth.TokenInfo, bucketName string) bool {
	if auth.CanSeeAllBuckets(info.Role) {
		return true
	}
	rec, err := s.resolveBucketForUser(info, bucketName)
	if err != nil {
		return false
	}
	return s.resolveBucketAccess(info, rec, true)
}

func (s *Server) isTenantAdmin(info auth.TokenInfo, tenantID string) bool {
	return auth.CanManageTenant(info.Role, s.userTenantMemberships(info.UserID), tenantID)
}

func (s *Server) tenantMemberOwnerSet(tenantID string) map[string]struct{} {
	members, _ := s.meta.ListTenantMembers(tenantID)
	out := make(map[string]struct{}, len(members))
	for _, m := range members {
		out[m.UserID] = struct{}{}
	}
	return out
}

func (s *Server) primaryTenantIDForUser(userID string) string {
	if user, err := s.meta.GetUser(userID); err == nil && metadata.IsTenantScoped(user.TenantID) {
		return user.TenantID
	}
	members, _ := s.meta.ListUserTenants(userID)
	for _, m := range members {
		if metadata.IsTenantScoped(m.TenantID) {
			return m.TenantID
		}
	}
	if user, err := s.meta.GetUser(userID); err == nil && user.TenantID != "" {
		return user.TenantID
	}
	return metadata.DefaultTenantID
}

func (s *Server) stampBucketOwnership(logicalBucket string, info auth.TokenInfo) {
	rec, err := s.resolveBucketForUser(info, logicalBucket)
	if err != nil {
		return
	}
	changed := false
	if rec.Owner == "" && info.Username != "" {
		rec.Owner = info.Username
		changed = true
	}
	if rec.OwnerID == "" && info.UserID != "" {
		rec.OwnerID = info.UserID
		changed = true
	}
	if user, err := s.meta.GetUser(info.UserID); err == nil {
		if rec.TeamID == "" && user.TeamID != "" {
			rec.TeamID = user.TeamID
			changed = true
		}
	}
	tenantID := s.primaryTenantIDForUser(info.UserID)
	if rec.TenantID != tenantID {
		rec.TenantID = tenantID
		changed = true
	}
	if changed {
		_ = s.meta.UpdateBucket(rec)
	}
}
