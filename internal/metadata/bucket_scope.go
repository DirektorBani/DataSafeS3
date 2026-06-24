package metadata

import (
	"strings"
)

type BucketScopeKind int

const (
	ScopeTenant BucketScopeKind = iota
	ScopeOwner
)

// BucketScope identifies the namespace for logical bucket name uniqueness.
type BucketScope struct {
	Kind     BucketScopeKind
	TenantID string
	OwnerID  string
}

// BucketAccessGrant is an explicit per-user permission on a tenant bucket.
type BucketAccessGrant struct {
	BucketKey string `json:"bucket_key"`
	UserID    string `json:"user_id"`
	CanRead   bool   `json:"can_read"`
	CanWrite  bool   `json:"can_write"`
}

// BucketPrefixAccessGrant limits access to objects under a prefix within a bucket.
type BucketPrefixAccessGrant struct {
	BucketKey string `json:"bucket_key"`
	UserID    string `json:"user_id"`
	Prefix    string `json:"prefix"`
	CanRead   bool   `json:"can_read"`
	CanWrite  bool   `json:"can_write"`
}

// NormalizeSharePrefix canonicalizes a folder prefix for grant matching (trailing slash).
func NormalizeSharePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	prefix = strings.TrimPrefix(prefix, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return prefix
}

// ObjectMatchesSharePrefix reports whether an object key is within a grant prefix.
func ObjectMatchesSharePrefix(objectKey, prefix string) bool {
	prefix = NormalizeSharePrefix(prefix)
	if prefix == "" {
		return true
	}
	return strings.HasPrefix(objectKey, prefix) || objectKey == strings.TrimSuffix(prefix, "/")
}

// IsTenantScoped reports whether a bucket belongs to a non-default tenant namespace.
func IsTenantScoped(tenantID string) bool {
	return tenantID != "" && tenantID != DefaultTenantID
}

// BucketScopeForRecord derives scope from bucket metadata.
func BucketScopeForRecord(tenantID, ownerID string) BucketScope {
	if IsTenantScoped(tenantID) {
		return BucketScope{Kind: ScopeTenant, TenantID: tenantID}
	}
	return BucketScope{Kind: ScopeOwner, OwnerID: ownerID}
}

// BucketScopeForUser picks the scope used when a user creates or resolves a bucket by logical name.
func BucketScopeForUser(userTenantID, userID string, tenantMemberships []TenantMemberRecord) BucketScope {
	if IsTenantScoped(userTenantID) {
		return BucketScope{Kind: ScopeTenant, TenantID: userTenantID}
	}
	for _, m := range tenantMemberships {
		if IsTenantScoped(m.TenantID) {
			return BucketScope{Kind: ScopeTenant, TenantID: m.TenantID}
		}
	}
	return BucketScope{Kind: ScopeOwner, OwnerID: userID}
}

// MakeStorageKey builds the internal storage path key for a new bucket.
func MakeStorageKey(scope BucketScope, name string) string {
	switch scope.Kind {
	case ScopeTenant:
		return "t__" + sanitizeScopePart(scope.TenantID) + "__" + sanitizeScopePart(name)
	default:
		if scope.OwnerID == "" {
			return name
		}
		return "o__" + sanitizeScopePart(scope.OwnerID) + "__" + sanitizeScopePart(name)
	}
}

// ScopeIndexKey is the secondary index key for scoped uniqueness lookups.
func ScopeIndexKey(scope BucketScope, name string) string {
	switch scope.Kind {
	case ScopeTenant:
		return "t\x00" + scope.TenantID + "\x00" + name
	default:
		return "o\x00" + scope.OwnerID + "\x00" + name
	}
}

// EffectiveStorageKey returns the key used for backend I/O.
func (b BucketRecord) EffectiveStorageKey() string {
	if b.StorageKey != "" {
		return b.StorageKey
	}
	return b.Name
}

// EffectiveTenantID returns the tenant namespace for a bucket, inferring from metadata or storage key.
func (b BucketRecord) EffectiveTenantID() string {
	if b.TenantID != "" {
		return b.TenantID
	}
	if tid := tenantIDFromStorageKey(b.StorageKey); tid != "" {
		return tid
	}
	return DefaultTenantID
}

func tenantIDFromStorageKey(storageKey string) string {
	const prefix = "t__"
	if !strings.HasPrefix(storageKey, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(storageKey, prefix)
	if i := strings.Index(rest, "__"); i > 0 {
		return rest[:i]
	}
	return ""
}

// LegacyBucket reports buckets created before scoped storage keys (storage_key == name).
func (b BucketRecord) LegacyBucket() bool {
	return b.StorageKey == "" || b.StorageKey == b.Name
}

func sanitizeScopePart(s string) string {
	return strings.NewReplacer(":", "_", "\\", "_", "/", "_", "\x00", "_").Replace(s)
}
