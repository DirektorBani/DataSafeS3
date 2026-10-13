package metadata

import "time"

// MFAFacts is the role information the 2FA policy decision needs.
type MFAFacts struct {
	Role             string
	BucketAdmin      bool
	TenantAdmin      bool
	BucketAdminSince *time.Time
	SubjectSince     *time.Time
}

// MFASubjectStore is the slice of MetadataStore used to decide who the policy covers.
type MFASubjectStore interface {
	ListBuckets() ([]BucketRecord, error)
	ListBucketAccessGrants(bucketKey string) ([]BucketAccessGrant, error)
	ListUserTenants(userID string) ([]TenantMemberRecord, error)
}

// MFASubjectFor reports whether user owns a bucket, holds an admin grant, or
// is a tenant administrator. Global-admin coverage is decided by the caller
// from Role.
func MFASubjectFor(store MFASubjectStore, user UserRecord) MFAFacts {
	facts := MFAFacts{
		Role:             user.Role,
		BucketAdminSince: user.BucketAdminSince,
		SubjectSince:     user.MFAPolicySince,
	}
	if members, err := store.ListUserTenants(user.ID); err == nil {
		for _, m := range members {
			if m.Role == TenantRoleAdmin {
				facts.TenantAdmin = true
				break
			}
		}
	}
	buckets, err := store.ListBuckets()
	if err != nil {
		return facts
	}
	for _, b := range buckets {
		if user.ID != "" && (b.OwnerID == user.ID || (b.OwnerID == "" && b.Owner == user.Username)) {
			facts.BucketAdmin = true
			return facts
		}
		grants, err := store.ListBucketAccessGrants(b.EffectiveStorageKey())
		if err != nil {
			continue
		}
		for _, g := range grants {
			if g.UserID == user.ID && g.Level == GrantLevelAdmin {
				facts.BucketAdmin = true
				return facts
			}
		}
	}
	return facts
}
