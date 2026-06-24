package auth

import "strings"

const (
	RoleAdministrator = "administrator"
	RoleOperator      = "operator"
	RoleUser          = "user"

	TenantRoleAdmin  = "tenant_admin"
	TenantRoleMember = "member"
	TenantRoleViewer = "viewer"
)

// TenantMembership is a user's role within a tenant (from tenant_members).
type TenantMembership struct {
	TenantID string
	Role     string
}

func IsAdmin(role string) bool {
	return role == RoleAdministrator
}

func IsOperatorOrAbove(role string) bool {
	return role == RoleAdministrator || role == RoleOperator
}

func CanManageUsers(role string) bool {
	return role == RoleAdministrator
}

func CanManageSettings(role string) bool {
	return role == RoleAdministrator
}

func CanSeeAllBuckets(role string) bool {
	return role == RoleAdministrator || role == RoleOperator
}

func CanSeeAllActivity(role string) bool {
	return role == RoleAdministrator
}

func CanManagePolicies(role string) bool {
	return role == RoleAdministrator
}

func CanSeeSystemUsage(role string) bool {
	return role == RoleAdministrator
}

func CanAccessFederation(role string) bool {
	return role == RoleAdministrator
}

func CanAccessCluster(role string) bool {
	return role == RoleAdministrator
}

func tenantMemberRole(memberships []TenantMembership, tenantID string) (string, bool) {
	if tenantID == "" {
		return "", false
	}
	for _, m := range memberships {
		if m.TenantID == tenantID {
			return m.Role, true
		}
	}
	return "", false
}

// IsTenantAdminRole reports whether a tenant_members.role value is tenant_admin.
func IsTenantAdminRole(role string) bool {
	return role == TenantRoleAdmin
}

// CanManageTenant reports whether a principal may administer a specific tenant
// (members, bucket access grants). Global administrators always can.
func CanManageTenant(globalRole string, memberships []TenantMembership, tenantID string) bool {
	if IsAdmin(globalRole) {
		return true
	}
	role, ok := tenantMemberRole(memberships, tenantID)
	return ok && role == TenantRoleAdmin
}

// CanAssignTenantRole reports whether a principal may assign a tenant_members role.
// Only global administrators may assign tenant_admin; others may assign member or viewer.
func CanAssignTenantRole(globalRole, tenantRole string) bool {
	switch tenantRole {
	case TenantRoleMember, TenantRoleViewer:
		return true
	case TenantRoleAdmin:
		return IsAdmin(globalRole)
	default:
		return false
	}
}

// CanManageAnyTenant reports whether a principal is tenant_admin of at least one tenant.
func CanManageAnyTenant(globalRole string, memberships []TenantMembership) bool {
	if IsAdmin(globalRole) {
		return true
	}
	for _, m := range memberships {
		if m.Role == TenantRoleAdmin {
			return true
		}
	}
	return false
}

// TenantRoleCanWrite reports whether a tenant member role allows mutating bucket contents.
func TenantRoleCanWrite(role string) bool {
	switch role {
	case TenantRoleAdmin, TenantRoleMember:
		return true
	default:
		return false
	}
}

// BucketGrant is a minimal grant view for RBAC checks (avoids importing metadata).
type BucketGrant struct {
	UserID   string
	CanRead  bool
	CanWrite bool
}

// PrefixGrant is folder-scoped access within a bucket.
type PrefixGrant struct {
	UserID   string
	Prefix   string
	CanRead  bool
	CanWrite bool
}

// GroupBucketAccess is group-based access for a single bucket.
type GroupBucketAccess struct {
	CanRead  bool
	CanWrite bool
}

// TenantBucketAccessInput collects inputs for tenant-scoped bucket access resolution.
type TenantBucketAccessInput struct {
	Role               string
	UserID             string
	Username           string
	TeamIDs            []string
	BucketOwnerID      string
	BucketOwner        string
	BucketTeamID       string
	BucketTenantID     string
	BucketKey          string
	UserTenants        []TenantMembership
	Grants              []BucketGrant
	PrefixGrants        []PrefixGrant
	HasGrants           bool
	HasPrefixGrants     bool
	GroupAccess         *GroupBucketAccess
	TenantHasGroups    bool
	BucketInTenantGroup bool
}

func grantAllows(grants []BucketGrant, userID string, write bool) bool {
	for _, g := range grants {
		if g.UserID != userID {
			continue
		}
		if write {
			return g.CanWrite
		}
		return g.CanRead || g.CanWrite
	}
	return false
}

func prefixGrantAllows(grants []PrefixGrant, userID, objectKey string, write bool) bool {
	for _, g := range grants {
		if g.UserID != userID {
			continue
		}
		if !objectKeyMatchesPrefix(objectKey, g.Prefix) {
			continue
		}
		if write {
			return g.CanWrite
		}
		return g.CanRead || g.CanWrite
	}
	return false
}

func userHasPrefixGrant(grants []PrefixGrant, userID string) bool {
	for _, g := range grants {
		if g.UserID == userID && (g.CanRead || g.CanWrite) {
			return true
		}
	}
	return false
}

func objectKeyMatchesPrefix(objectKey, prefix string) bool {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return true
	}
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.HasPrefix(objectKey, prefix) || objectKey == strings.TrimSuffix(prefix, "/")
}

// ResolveTenantBucketAccess applies union of grants, groups, and default tenant membership.
func ResolveTenantBucketAccess(in TenantBucketAccessInput, write bool) bool {
	if CanSeeAllBuckets(in.Role) {
		return true
	}
	if in.BucketOwnerID != "" && in.BucketOwnerID == in.UserID {
		return true
	}
	if in.BucketOwner != "" && in.BucketOwner == in.Username {
		return true
	}
	if tenantRole, ok := tenantMemberRole(in.UserTenants, in.BucketTenantID); ok && tenantRole == TenantRoleAdmin {
		return true
	}
	if in.HasGrants && grantAllows(in.Grants, in.UserID, write) {
		return true
	}
	if !write && in.HasPrefixGrants && userHasPrefixGrant(in.PrefixGrants, in.UserID) {
		return true
	}
	if in.GroupAccess != nil {
		if write {
			return in.GroupAccess.CanWrite
		}
		return in.GroupAccess.CanRead
	}
	regulated := in.HasGrants || in.HasPrefixGrants || (in.TenantHasGroups && in.BucketInTenantGroup)
	if regulated {
		return false
	}
	if in.TenantHasGroups {
		return false
	}
	if _, ok := tenantMemberRole(in.UserTenants, in.BucketTenantID); ok {
		if write {
			role, _ := tenantMemberRole(in.UserTenants, in.BucketTenantID)
			return TenantRoleCanWrite(role)
		}
		return true
	}
	if in.BucketTeamID != "" {
		for _, tid := range in.TeamIDs {
			if tid == in.BucketTeamID {
				if write {
					return true
				}
				return true
			}
		}
	}
	return false
}

// ResolveObjectKeyAccess checks read/write for a specific object key (prefix grants).
func ResolveObjectKeyAccess(in TenantBucketAccessInput, objectKey string, write bool) bool {
	if ResolveTenantBucketAccess(in, write) {
		if !in.HasPrefixGrants || grantAllows(in.Grants, in.UserID, write) {
			return true
		}
		if !userHasPrefixGrant(in.PrefixGrants, in.UserID) {
			return true
		}
	}
	if in.HasPrefixGrants {
		return prefixGrantAllows(in.PrefixGrants, in.UserID, objectKey, write)
	}
	return false
}

// CanAccessBucketWithGrants applies explicit grants when the bucket has regulated access.
func CanAccessBucketWithGrants(role, userID, username, bucketOwnerID, bucketOwner, bucketTenantID string, userTenants []TenantMembership, grants []BucketGrant, write bool) bool {
	if CanSeeAllBuckets(role) {
		return true
	}
	if bucketOwnerID != "" && bucketOwnerID == userID {
		return true
	}
	if bucketOwner != "" && bucketOwner == username {
		return true
	}
	if tenantRole, ok := tenantMemberRole(userTenants, bucketTenantID); ok && tenantRole == TenantRoleAdmin {
		return true
	}
	for _, g := range grants {
		if g.UserID != userID {
			continue
		}
		if write {
			return g.CanWrite
		}
		return g.CanRead || g.CanWrite
	}
	return false
}

// CanAccessBucket reports whether a principal may read a bucket by ownership, team, or tenant membership.
// Administrators and operators bypass the check (see all buckets).
func CanAccessBucket(role, userID, username string, userTeamIDs []string, bucketOwnerID, bucketOwner, bucketTeamID, bucketTenantID string, userTenants []TenantMembership) bool {
	if CanSeeAllBuckets(role) {
		return true
	}
	if bucketOwnerID != "" && bucketOwnerID == userID {
		return true
	}
	if bucketOwner != "" && bucketOwner == username {
		return true
	}
	if bucketTeamID != "" {
		for _, tid := range userTeamIDs {
			if tid == bucketTeamID {
				return true
			}
		}
	}
	if _, ok := tenantMemberRole(userTenants, bucketTenantID); ok {
		return true
	}
	return false
}

// CanWriteBucket reports whether a principal may modify bucket contents or settings.
func CanWriteBucket(role, userID, username string, userTeamIDs []string, bucketOwnerID, bucketOwner, bucketTeamID, bucketTenantID string, userTenants []TenantMembership) bool {
	if !CanAccessBucket(role, userID, username, userTeamIDs, bucketOwnerID, bucketOwner, bucketTeamID, bucketTenantID, userTenants) {
		return false
	}
	if CanSeeAllBuckets(role) {
		return true
	}
	if bucketOwnerID != "" && bucketOwnerID == userID {
		return true
	}
	if bucketOwner != "" && bucketOwner == username {
		return true
	}
	if bucketTeamID != "" {
		for _, tid := range userTeamIDs {
			if tid == bucketTeamID {
				return true
			}
		}
	}
	if tenantRole, ok := tenantMemberRole(userTenants, bucketTenantID); ok {
		return TenantRoleCanWrite(tenantRole)
	}
	return false
}

// IsS3WriteAction reports whether an IAM-style S3 action mutates data.
func IsS3WriteAction(action string) bool {
	if action == "" {
		return false
	}
	switch action {
	case "s3:PutObject", "s3:DeleteObject", "s3:CreateBucket", "s3:DeleteBucket",
		"s3:PutBucketVersioning", "s3:PutLifecycleConfiguration", "s3:PutObjectTagging",
		"s3:AbortMultipartUpload", "s3:CompleteMultipartUpload":
		return true
	}
	return strings.HasPrefix(action, "s3:Put") || strings.HasPrefix(action, "s3:Delete")
}
