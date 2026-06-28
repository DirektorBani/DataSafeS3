package api

import (
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

type bucketAccessGrantView struct {
	UserID   string `json:"user_id"`
	Username string `json:"username,omitempty"`
	Prefix   string `json:"prefix,omitempty"`
	CanRead  bool   `json:"can_read"`
	CanWrite bool   `json:"can_write"`
}

type bucketAccessInfo struct {
	Ownership      string   `json:"ownership"`
	CanRead        bool     `json:"can_read"`
	CanWrite       bool     `json:"can_write"`
	SharedBy       *string  `json:"shared_by"`
	SharedPrefixes []string `json:"shared_prefixes,omitempty"`
}

func (s *Server) canManageBucketGrants(info auth.TokenInfo, rec metadata.BucketRecord) bool {
	if auth.IsAdmin(info.Role) {
		return true
	}
	if rec.OwnerID == info.UserID || (rec.Owner != "" && rec.Owner == info.Username) {
		return true
	}
	tid := rec.EffectiveTenantID()
	if metadata.IsTenantScoped(tid) {
		return s.isTenantAdmin(info, tid)
	}
	return false
}

func (s *Server) tenantIDSetForUser(userID string) map[string]struct{} {
	out := map[string]struct{}{}
	if user, err := s.meta.GetUser(userID); err == nil && user.TenantID != "" {
		out[user.TenantID] = struct{}{}
	}
	for _, m := range s.userTenantMemberships(userID) {
		if m.TenantID != "" {
			out[m.TenantID] = struct{}{}
		}
	}
	return out
}

// granteeAllowedForBucket enforces MVP shareability: tenant members for tenant buckets;
// for personal buckets grantee must share at least one tenant_members tenant with the bucket owner,
// or caller is administrator.
func (s *Server) granteeAllowedForBucket(rec metadata.BucketRecord, granteeID string, caller auth.TokenInfo) error {
	grantee, err := s.meta.GetUser(granteeID)
	if err != nil || grantee.Status != metadata.StatusActive {
		return errGranteeInvalid
	}
	if granteeID == rec.OwnerID {
		return errGranteeIsOwner
	}
	tid := rec.EffectiveTenantID()
	if metadata.IsTenantScoped(tid) {
		if member, merr := s.meta.GetTenantMember(tid, granteeID); merr != nil || member.Role == "" {
			return errGranteeNotInTenant
		}
		return nil
	}
	if auth.IsAdmin(caller.Role) {
		return nil
	}
	ownerID := rec.OwnerID
	if ownerID == "" && rec.Owner != "" {
		if u, uerr := s.meta.GetUserByUsername(rec.Owner); uerr == nil {
			ownerID = u.ID
		}
	}
	if ownerID == "" {
		return errGranteeNotShareable
	}
	ownerTenants := s.tenantIDSetForUser(ownerID)
	granteeTenants := s.tenantIDSetForUser(granteeID)
	for tid := range ownerTenants {
		if _, ok := granteeTenants[tid]; ok {
			return nil
		}
	}
	return errGranteeNotShareable
}

var (
	errGranteeInvalid      = &grantError{"user not found or inactive"}
	errGranteeIsOwner      = &grantError{"cannot grant bucket owner"}
	errGranteeNotInTenant  = &grantError{"user not in tenant"}
	errGranteeNotShareable = &grantError{"user not shareable"}
)

type grantError struct{ msg string }

func (e *grantError) Error() string { return e.msg }

func (s *Server) listBucketAccessGrants(rec metadata.BucketRecord) ([]bucketAccessGrantView, error) {
	grants, err := s.meta.ListBucketAccessGrants(rec.EffectiveStorageKey())
	if err != nil {
		return nil, err
	}
	out := make([]bucketAccessGrantView, 0, len(grants))
	for _, g := range grants {
		item := bucketAccessGrantView{UserID: g.UserID, CanRead: g.CanRead, CanWrite: g.CanWrite}
		if u, uerr := s.meta.GetUser(g.UserID); uerr == nil {
			item.Username = u.Username
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Server) listBucketPrefixAccessGrants(rec metadata.BucketRecord) ([]bucketAccessGrantView, error) {
	grants, err := s.meta.ListBucketPrefixAccessGrants(rec.EffectiveStorageKey())
	if err != nil {
		return nil, err
	}
	out := make([]bucketAccessGrantView, 0, len(grants))
	for _, g := range grants {
		item := bucketAccessGrantView{UserID: g.UserID, Prefix: g.Prefix, CanRead: g.CanRead, CanWrite: g.CanWrite}
		if u, uerr := s.meta.GetUser(g.UserID); uerr == nil {
			item.Username = u.Username
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Server) replaceBucketPrefixAccessGrants(rec metadata.BucketRecord, grants []metadata.BucketPrefixAccessGrant) error {
	return s.meta.ReplaceBucketPrefixAccessGrants(rec.EffectiveStorageKey(), grants)
}

func (s *Server) replaceBucketAccessGrants(rec metadata.BucketRecord, grants []metadata.BucketAccessGrant) error {
	return s.meta.ReplaceBucketAccessGrants(rec.EffectiveStorageKey(), grants)
}

func (s *Server) bucketAccessForUser(info auth.TokenInfo, rec metadata.BucketRecord) bucketAccessInfo {
	ownership, sharedBy := s.bucketOwnership(info, rec)
	prefixes := s.sharedPrefixesForUser(info, rec)
	return bucketAccessInfo{
		Ownership:      ownership,
		CanRead:        s.resolveBucketAccess(info, rec, false),
		CanWrite:       s.resolveBucketAccess(info, rec, true),
		SharedBy:       sharedBy,
		SharedPrefixes: prefixes,
	}
}

func (s *Server) sharedPrefixesForUser(info auth.TokenInfo, rec metadata.BucketRecord) []string {
	hasFull := false
	for _, g := range s.bucketGrantsFor(rec) {
		if g.UserID == info.UserID && (g.CanRead || g.CanWrite) {
			hasFull = true
			break
		}
	}
	if hasFull {
		return nil
	}
	var out []string
	for _, g := range s.bucketPrefixGrantsFor(rec) {
		if g.UserID == info.UserID && (g.CanRead || g.CanWrite) && g.Prefix != "" {
			out = append(out, g.Prefix)
		}
	}
	return out
}

func (s *Server) bucketOwnership(info auth.TokenInfo, rec metadata.BucketRecord) (string, *string) {
	if rec.OwnerID == info.UserID || (rec.Owner != "" && rec.Owner == info.Username) {
		return "owned", nil
	}
	bucketKey := rec.EffectiveStorageKey()
	for _, g := range s.bucketGrantsFor(rec) {
		if g.UserID == info.UserID && (g.CanRead || g.CanWrite) {
			owner := rec.Owner
			if owner == "" && rec.OwnerID != "" {
				if u, err := s.meta.GetUser(rec.OwnerID); err == nil {
					owner = u.Username
				}
			}
			if owner != "" {
				return "shared", &owner
			}
			return "shared", nil
		}
	}
	for _, g := range s.bucketPrefixGrantsFor(rec) {
		if g.UserID == info.UserID && (g.CanRead || g.CanWrite) {
			owner := rec.Owner
			if owner == "" && rec.OwnerID != "" {
				if u, err := s.meta.GetUser(rec.OwnerID); err == nil {
					owner = u.Username
				}
			}
			if owner != "" {
				return "shared", &owner
			}
			return "shared", nil
		}
	}
	_ = bucketKey
	return "tenant", nil
}

func (s *Server) shareableUsersForBucket(rec metadata.BucketRecord, ownerID, query string, limit int) ([]bucketAccessGrantView, error) {
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	query = strings.ToLower(strings.TrimSpace(query))
	seen := map[string]struct{}{}
	var out []bucketAccessGrantView

	addUser := func(userID string) {
		if userID == ownerID {
			return
		}
		if _, ok := seen[userID]; ok {
			return
		}
		u, err := s.meta.GetUser(userID)
		if err != nil || u.Status != metadata.StatusActive {
			return
		}
		if query != "" && !strings.Contains(strings.ToLower(u.Username), query) {
			return
		}
		seen[userID] = struct{}{}
		out = append(out, bucketAccessGrantView{UserID: u.ID, Username: u.Username})
	}

	tid := rec.EffectiveTenantID()
	if metadata.IsTenantScoped(tid) {
		members, err := s.meta.ListTenantMembers(tid)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			addUser(m.UserID)
			if len(out) >= limit {
				return out, nil
			}
		}
		return out, nil
	}
	if ownerID == "" {
		return out, nil
	}
	ownerTenants := s.tenantIDSetForUser(ownerID)
	for tenantID := range ownerTenants {
		members, err := s.meta.ListTenantMembers(tenantID)
		if err != nil {
			continue
		}
		for _, m := range members {
			addUser(m.UserID)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func grantErrorStatus(err error) int {
	switch err {
	case errGranteeInvalid, errGranteeNotInTenant, errGranteeNotShareable, errGranteeIsOwner:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
