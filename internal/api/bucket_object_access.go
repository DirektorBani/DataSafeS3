package api

import (
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) canAccessObjectKey(info auth.TokenInfo, bucketName, objectKey string) bool {
	rec, err := s.resolveBucketForUser(info, bucketName)
	if err != nil {
		return false
	}
	in := s.tenantBucketAccessInput(info, rec)
	return auth.ResolveObjectKeyAccess(in, objectKey, false)
}

func (s *Server) canWriteObjectKey(info auth.TokenInfo, bucketName, objectKey string) bool {
	rec, err := s.resolveBucketForUser(info, bucketName)
	if err != nil {
		return false
	}
	in := s.tenantBucketAccessInput(info, rec)
	return auth.ResolveObjectKeyAccess(in, objectKey, true)
}

func (s *Server) userHasPrefixOnlyAccess(info auth.TokenInfo, rec metadata.BucketRecord) bool {
	if rec.OwnerID == info.UserID || (rec.Owner != "" && rec.Owner == info.Username) {
		return false
	}
	if auth.IsAdmin(info.Role) {
		return false
	}
	in := s.tenantBucketAccessInput(info, rec)
	for _, g := range in.Grants {
		if g.UserID == info.UserID && (g.CanRead || g.CanWrite) {
			return false
		}
	}
	for _, g := range in.PrefixGrants {
		if g.UserID == info.UserID && (g.CanRead || g.CanWrite) {
			return true
		}
	}
	return false
}

// allowedListPrefix returns an effective list prefix for prefix-only grantees.
func (s *Server) allowedListPrefix(info auth.TokenInfo, rec metadata.BucketRecord, requested string) (string, bool) {
	if !s.userHasPrefixOnlyAccess(info, rec) {
		return requested, true
	}
	in := s.tenantBucketAccessInput(info, rec)
	var allowed []string
	for _, g := range in.PrefixGrants {
		if g.UserID != info.UserID || !(g.CanRead || g.CanWrite) {
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

func (s *Server) filterObjectsForPrefixAccess(info auth.TokenInfo, rec metadata.BucketRecord, objs []metadata.ObjectRecord) []metadata.ObjectRecord {
	if !s.userHasPrefixOnlyAccess(info, rec) {
		return objs
	}
	in := s.tenantBucketAccessInput(info, rec)
	out := make([]metadata.ObjectRecord, 0, len(objs))
	for _, o := range objs {
		if auth.ResolveObjectKeyAccess(in, o.Key, false) {
			out = append(out, o)
		}
	}
	return out
}

func (s *Server) prefixGrantFolderRoots(info auth.TokenInfo, rec metadata.BucketRecord) []string {
	if !s.userHasPrefixOnlyAccess(info, rec) {
		return nil
	}
	in := s.tenantBucketAccessInput(info, rec)
	seen := map[string]struct{}{}
	var roots []string
	for _, g := range in.PrefixGrants {
		if g.UserID != info.UserID || !(g.CanRead || g.CanWrite) {
			continue
		}
		p := metadata.NormalizeSharePrefix(g.Prefix)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		roots = append(roots, p)
	}
	return roots
}
