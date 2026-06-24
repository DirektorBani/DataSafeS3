package s3

import (
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Service) grantsForRBAC(grants []metadata.BucketAccessGrant) []auth.BucketGrant {
	out := make([]auth.BucketGrant, 0, len(grants))
	for _, g := range grants {
		out = append(out, auth.BucketGrant{UserID: g.UserID, CanRead: g.CanRead, CanWrite: g.CanWrite})
	}
	return out
}

func (s *Service) bucketGrantsFor(rec metadata.BucketRecord) []metadata.BucketAccessGrant {
	grants, _ := s.Meta.ListBucketAccessGrants(rec.EffectiveStorageKey())
	return grants
}

func (s *Service) bucketUsesGrants(rec metadata.BucketRecord) bool {
	n, _ := s.Meta.CountBucketAccessGrants(rec.EffectiveStorageKey())
	return n > 0
}

func (s *Service) tenantGroupAccessForUser(userID, bucketKey string) *auth.GroupBucketAccess {
	accesses, _ := s.Meta.ListUserGroupBucketAccess(userID)
	for _, a := range accesses {
		if a.BucketKey == bucketKey {
			return &auth.GroupBucketAccess{CanRead: a.CanRead, CanWrite: a.CanWrite}
		}
	}
	return nil
}

func (s *Service) tenantHasGroups(tenantID string) bool {
	n, _ := s.Meta.CountTenantGroups(tenantID)
	return n > 0
}

func (s *Service) bucketInTenantGroup(tenantID, bucketKey string) bool {
	keys, _ := s.Meta.ListTenantGroupBucketKeys(tenantID)
	for _, k := range keys {
		if k == bucketKey {
			return true
		}
	}
	return false
}

func (s *Service) tenantAdminIDs(memberships []auth.TenantMembership) []string {
	var out []string
	for _, m := range memberships {
		if m.Role == auth.TenantRoleAdmin {
			out = append(out, m.TenantID)
		}
	}
	return out
}

func (s *Service) tenantsWithGroupsForUser(memberships []auth.TenantMembership) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range memberships {
		if s.tenantHasGroups(m.TenantID) {
			out[m.TenantID] = struct{}{}
		}
	}
	return out
}

func (s *Service) groupBucketKeysForUser(userID string) map[string]struct{} {
	out := map[string]struct{}{}
	accesses, _ := s.Meta.ListUserGroupBucketAccess(userID)
	for _, a := range accesses {
		out[a.BucketKey] = struct{}{}
	}
	return out
}

func (s *Service) prefixGrantsForRBAC(grants []metadata.BucketPrefixAccessGrant) []auth.PrefixGrant {
	out := make([]auth.PrefixGrant, 0, len(grants))
	for _, g := range grants {
		out = append(out, auth.PrefixGrant{UserID: g.UserID, Prefix: g.Prefix, CanRead: g.CanRead, CanWrite: g.CanWrite})
	}
	return out
}

func (s *Service) bucketPrefixGrantsFor(rec metadata.BucketRecord) []metadata.BucketPrefixAccessGrant {
	grants, _ := s.Meta.ListBucketPrefixAccessGrants(rec.EffectiveStorageKey())
	return grants
}

func (s *Service) bucketUsesPrefixGrants(rec metadata.BucketRecord) bool {
	n, _ := s.Meta.CountBucketPrefixAccessGrants(rec.EffectiveStorageKey())
	return n > 0
}

func (s *Service) tenantBucketAccessInput(role, userID, username string, teamIDs []string, rec metadata.BucketRecord) auth.TenantBucketAccessInput {
	grants := s.bucketGrantsFor(rec)
	prefixGrants := s.bucketPrefixGrantsFor(rec)
	bucketKey := rec.EffectiveStorageKey()
	return auth.TenantBucketAccessInput{
		Role: role, UserID: userID, Username: username, TeamIDs: teamIDs,
		BucketOwnerID: rec.OwnerID, BucketOwner: rec.Owner, BucketTeamID: rec.TeamID,
		BucketTenantID: rec.TenantID, BucketKey: bucketKey,
		UserTenants: s.userTenantMemberships(userID), Grants: s.grantsForRBAC(grants),
		PrefixGrants: s.prefixGrantsForRBAC(prefixGrants),
		HasGrants: s.bucketUsesGrants(rec), HasPrefixGrants: s.bucketUsesPrefixGrants(rec),
		GroupAccess: s.tenantGroupAccessForUser(userID, bucketKey),
		TenantHasGroups: s.tenantHasGroups(rec.TenantID), BucketInTenantGroup: s.bucketInTenantGroup(rec.TenantID, bucketKey),
	}
}

func (s *Service) grantBucketKeysForUser(userID string) map[string]struct{} {
	out := map[string]struct{}{}
	buckets, _ := s.Meta.ListBuckets()
	for _, b := range buckets {
		grants, _ := s.Meta.ListBucketAccessGrants(b.EffectiveStorageKey())
		for _, g := range grants {
			if g.UserID == userID && (g.CanRead || g.CanWrite) {
				out[b.EffectiveStorageKey()] = struct{}{}
			}
		}
		prefixGrants, _ := s.Meta.ListBucketPrefixAccessGrants(b.EffectiveStorageKey())
		for _, g := range prefixGrants {
			if g.UserID == userID && (g.CanRead || g.CanWrite) {
				out[b.EffectiveStorageKey()] = struct{}{}
			}
		}
	}
	return out
}

func (s *Service) resolveBucketAccess(role, userID, username string, teamIDs []string, rec metadata.BucketRecord, write bool) bool {
	in := s.tenantBucketAccessInput(role, userID, username, teamIDs, rec)
	if in.BucketTenantID != "" || in.HasGrants || in.HasPrefixGrants || in.TenantHasGroups {
		return auth.ResolveTenantBucketAccess(in, write)
	}
	if write {
		return auth.CanWriteBucket(role, userID, username, teamIDs, rec.OwnerID, rec.Owner, rec.TeamID, rec.TenantID, in.UserTenants)
	}
	return auth.CanAccessBucket(role, userID, username, teamIDs, rec.OwnerID, rec.Owner, rec.TeamID, rec.TenantID, in.UserTenants)
}

func (s *Service) resolveObjectKeyAccess(role, userID, username string, teamIDs []string, rec metadata.BucketRecord, objectKey string, write bool) bool {
	in := s.tenantBucketAccessInput(role, userID, username, teamIDs, rec)
	return auth.ResolveObjectKeyAccess(in, objectKey, write)
}

func (s *Service) accessKeyIdentity(ak metadata.AccessKeyRecord) (role, userID, username string, teamIDs []string) {
	userID = ak.OwnerID
	username = ak.Owner
	role = metadata.RoleUser
	if userID != "" {
		if u, err := s.Meta.GetUser(userID); err == nil {
			role = u.Role
			if username == "" {
				username = u.Username
			}
		}
	} else if username != "" {
		if u, err := s.Meta.GetUserByUsername(username); err == nil {
			userID = u.ID
			role = u.Role
		}
	}
	teamIDs = s.userTeamIDs(userID)
	return role, userID, username, teamIDs
}

func (s *Service) objectVisibleToAccessKey(ak metadata.AccessKeyRecord, rec metadata.BucketRecord, objectKey string) bool {
	role, userID, username, teamIDs := s.accessKeyIdentity(ak)
	if !s.resolveBucketAccess(role, userID, username, teamIDs, rec, false) {
		return false
	}
	return s.resolveObjectKeyAccess(role, userID, username, teamIDs, rec, objectKey, false)
}

func (s *Service) objectWritableByAccessKey(ak metadata.AccessKeyRecord, rec metadata.BucketRecord, objectKey string) bool {
	role, userID, username, teamIDs := s.accessKeyIdentity(ak)
	if !s.resolveBucketAccess(role, userID, username, teamIDs, rec, true) {
		return false
	}
	return s.resolveObjectKeyAccess(role, userID, username, teamIDs, rec, objectKey, true)
}

func (s *Service) FilterObjectsForAccessKey(accessKey, storageKey string, objs []metadata.ObjectRecord) []metadata.ObjectRecord {
	rec, err := s.Meta.GetBucketByKey(storageKey)
	if err != nil {
		return objs
	}
	ak, err := s.Meta.GetAccessKey(accessKey)
	if err != nil {
		return objs
	}
	role, userID, username, teamIDs := s.accessKeyIdentity(ak)
	in := s.tenantBucketAccessInput(role, userID, username, teamIDs, rec)
	if !prefixOnlyAccessFromInput(in) {
		return objs
	}
	out := make([]metadata.ObjectRecord, 0, len(objs))
	for _, o := range objs {
		if auth.ResolveObjectKeyAccess(in, o.Key, false) {
			out = append(out, o)
		}
	}
	return out
}

func prefixOnlyAccessFromInput(in auth.TenantBucketAccessInput) bool {
	if in.BucketOwnerID == in.UserID || (in.BucketOwner != "" && in.BucketOwner == in.Username) {
		return false
	}
	if auth.IsAdmin(in.Role) {
		return false
	}
	for _, g := range in.Grants {
		if g.UserID == in.UserID && (g.CanRead || g.CanWrite) {
			return false
		}
	}
	for _, g := range in.PrefixGrants {
		if g.UserID == in.UserID && (g.CanRead || g.CanWrite) {
			return true
		}
	}
	return false
}

// EffectiveListPrefixForAccessKey returns the list prefix allowed for prefix-only grantees.
func (s *Service) EffectiveListPrefixForAccessKey(accessKey, storageKey, requested string) (string, bool) {
	rec, err := s.Meta.GetBucketByKey(storageKey)
	if err != nil {
		return requested, true
	}
	ak, err := s.Meta.GetAccessKey(accessKey)
	if err != nil {
		return requested, true
	}
	role, userID, username, teamIDs := s.accessKeyIdentity(ak)
	in := s.tenantBucketAccessInput(role, userID, username, teamIDs, rec)
	if !prefixOnlyAccessFromInput(in) {
		return requested, true
	}
	var allowed []string
	for _, g := range in.PrefixGrants {
		if g.UserID != userID || !(g.CanRead || g.CanWrite) {
			continue
		}
		allowed = append(allowed, metadata.NormalizeSharePrefix(g.Prefix))
	}
	if len(allowed) == 0 {
		return "", false
	}
	req := metadata.NormalizeSharePrefix(requested)
	if req == "" {
		return "", true
	}
	for _, p := range allowed {
		if strings.HasPrefix(req, p) || strings.HasPrefix(p, req) {
			return req, true
		}
	}
	return "", false
}

func (s *Service) userTeamIDs(userID string) []string {
	extra, _ := s.Meta.ListUserTeamIDs(userID)
	if u, err := s.Meta.GetUser(userID); err == nil {
		return metadata.MergeTeamIDs(u.TeamID, extra)
	}
	return extra
}
