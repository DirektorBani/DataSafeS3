package s3

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/policy"
)

const (
	// ShareDisabledMessage is the S3 and admin-API text when sharing is off.
	ShareDisabledMessage   = "File sharing for this bucket is disabled by an administrator"
	ShareDisabledMessageRU = "Обмен файлами в этом бакете отключён администратором"
	PresignDisabledMessage = "Presigned URLs are disabled for this bucket"
)

var (
	ErrPublicBlocked   = errors.New("public access blocked")
	ErrPublicAmbiguous = errors.New("ambiguous public bucket")
)

func (s *Service) policyDenies(rec metadata.BucketRecord, action, key, principal string, ak *metadata.AccessKeyRecord) bool {
	if strings.TrimSpace(rec.Policy) == "" {
		return false
	}
	resource := policy.BucketARN(rec.Name)
	if key != "" {
		resource = policy.ObjectARN(rec.Name, key)
	}
	ctx := policy.EvalContext{
		Action:     action,
		Resource:   resource,
		Principals: []string{principal, policy.AccessKeyARN(principal)},
	}
	if ak != nil && ak.Owner != "" {
		ctx.Principals = append(ctx.Principals, ak.Owner, policy.UserARN(ak.Owner))
	}
	return policy.Denies(rec.Policy, ctx)
}

func (s *Service) EffectiveSharing(rec metadata.BucketRecord) metadata.EffectiveSharing {
	global, _ := s.Meta.GetSharingPolicy(metadata.SharingScopeGlobal, "")
	tenant, _ := s.Meta.GetSharingPolicy(metadata.SharingScopeTenant, rec.EffectiveTenantID())
	bucket, _ := s.Meta.GetSharingPolicy(metadata.SharingScopeBucket, rec.EffectiveStorageKey())
	return metadata.EffectiveSharingFrom(global, tenant, bucket)
}

// ResolvePublicRead picks the single bucket an anonymous request may read.
// More than one match is ErrPublicAmbiguous. Block Public Access is ErrPublicBlocked.
func (s *Service) ResolvePublicRead(name, tenantHint, sourceIP string, secure bool, key string) (metadata.BucketRecord, error) {
	buckets, err := s.Meta.ListBuckets()
	if err != nil {
		return metadata.BucketRecord{}, err
	}
	var matches []metadata.BucketRecord
	for _, rec := range buckets {
		if rec.Name != name {
			continue
		}
		if tenantHint != "" && rec.EffectiveTenantID() != tenantHint {
			continue
		}
		if !s.anonymousCandidate(rec, key, sourceIP, secure) {
			continue
		}
		matches = append(matches, rec)
	}
	if len(matches) == 0 {
		return metadata.BucketRecord{}, metadata.ErrNotFound
	}
	if len(matches) > 1 {
		return metadata.BucketRecord{}, ErrPublicAmbiguous
	}
	rec := matches[0]
	if s.EffectiveSharing(rec).PublicAccessBlocked {
		return rec, ErrPublicBlocked
	}
	return rec, nil
}

func (s *Service) anonymousCandidate(rec metadata.BucketRecord, key, sourceIP string, secure bool) bool {
	if key == "" {
		key = "object"
	}
	ctx := policy.EvalContext{
		Action:          "s3:GetObject",
		Resource:        policy.ObjectARN(rec.Name, key),
		Anonymous:       true,
		SourceIP:        sourceIP,
		SecureTransport: secure,
	}
	if strings.TrimSpace(rec.Policy) != "" {
		if policy.Denies(rec.Policy, ctx) {
			return false
		}
		if policy.EvaluateContext(rec.Policy, ctx) {
			return true
		}
	}
	return rec.Visibility == "public-read"
}

// PresignBlockReason is empty when a presigned request may proceed.
func (s *Service) PresignBlockReason(accessKey, logicalBucket string) string {
	_, rec, err := s.ResolveBucketKey(accessKey, logicalBucket)
	if err != nil {
		return ""
	}
	eff := s.EffectiveSharing(rec)
	if !eff.ShareLinksAllowed {
		return ShareDisabledMessage
	}
	if !eff.PresignAllowed {
		return PresignDisabledMessage
	}
	return ""
}

func (s *Service) CallerIsBucketAdmin(accessKey, logicalBucket string) bool {
	if accessKey == "" {
		return false
	}
	if accessKey == s.OwnerKey {
		return true
	}
	_, rec, err := s.ResolveBucketKey(accessKey, logicalBucket)
	if err != nil {
		return false
	}
	ak, err := s.Meta.GetAccessKey(accessKey)
	if err != nil {
		return false
	}
	role, userID, username, _ := s.accessKeyIdentity(ak)
	if role == metadata.RoleAdministrator {
		return true
	}
	if rec.OwnerID != "" && rec.OwnerID == userID {
		return true
	}
	if rec.Owner != "" && rec.Owner == username {
		return true
	}
	if s.tenantAdmin(userID, rec.EffectiveTenantID()) {
		return true
	}
	return auth.GrantIsAdmin(s.grantsForRBAC(s.bucketGrantsFor(rec)), userID)
}

func (s *Service) tenantAdmin(userID, tenantID string) bool {
	if userID == "" || tenantID == "" {
		return false
	}
	for _, m := range s.userTenantMemberships(userID) {
		if m.TenantID == tenantID && m.Role == auth.TenantRoleAdmin {
			return true
		}
	}
	return false
}

func requestSourceIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func tenantHint(r *http.Request) string {
	if h := strings.TrimSpace(r.Header.Get("X-DataSafe-Tenant")); h != "" {
		return h
	}
	return strings.TrimSpace(r.URL.Query().Get("tenant"))
}
