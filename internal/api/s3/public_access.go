package s3

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strconv"
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

func (s *Service) policyDenies(rec metadata.BucketRecord, action, key, principal string, ak *metadata.AccessKeyRecord, sourceIP string, secure bool) bool {
	if strings.TrimSpace(rec.Policy) == "" {
		return false
	}
	resource := policy.BucketARN(rec.Name)
	if key != "" {
		resource = policy.ObjectARN(rec.Name, key)
	}
	ctx := policy.EvalContext{
		Action:          action,
		Resource:        resource,
		Principals:      []string{principal, policy.AccessKeyARN(principal)},
		SourceIP:        sourceIP,
		SecureTransport: secure,
	}
	if ak != nil && ak.Owner != "" {
		ctx.Principals = append(ctx.Principals, ak.Owner, policy.UserARN(ak.Owner))
	}
	return policy.Denies(rec.Policy, ctx)
}

func policyBindsAccessKey(policyJSON, accessKey string) bool {
	policyJSON = strings.TrimSpace(policyJSON)
	if policyJSON == "" || accessKey == "" {
		return false
	}
	var doc policy.Document
	if json.Unmarshal([]byte(policyJSON), &doc) != nil {
		return false
	}
	want := policy.AccessKeyARN(accessKey)
	for _, st := range doc.Statement {
		if principalExact(st.Principal, accessKey) || principalExact(st.Principal, want) {
			return true
		}
	}
	return false
}

func principalExact(raw json.RawMessage, want string) bool {
	raw = bytesTrimJSON(raw)
	if len(raw) == 0 || string(raw) == "null" || want == "" {
		return false
	}
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		return asString == want
	}
	var box struct {
		AWS json.RawMessage `json:"AWS"`
	}
	if json.Unmarshal(raw, &box) != nil {
		return false
	}
	var one string
	if json.Unmarshal(box.AWS, &one) == nil {
		return one == want
	}
	var many []string
	if json.Unmarshal(box.AWS, &many) != nil {
		return false
	}
	for _, p := range many {
		if p == want {
			return true
		}
	}
	return false
}

func bytesTrimJSON(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
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

// requestSourceIP is the address aws:SourceIp sees. It is the TCP peer.
// X-Forwarded-For is used only when that peer is inside STORAGE_TRUSTED_PROXIES,
// and then the rightmost hop that is not itself a trusted proxy. A header from
// any other peer is ignored.
func requestSourceIP(r *http.Request) string {
	remote := peerIP(r.RemoteAddr)
	if !ipTrusted(remote) {
		return remote
	}
	if client := forwardedClientIP(r.Header.Get("X-Forwarded-For")); client != "" {
		return client
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" && net.ParseIP(xri) != nil && !ipTrusted(xri) {
		return xri
	}
	return remote
}

func peerIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	return strings.Trim(host, "[]")
}

func forwardedClientIP(xff string) string {
	parts := strings.Split(xff, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if net.ParseIP(ip) == nil {
			continue
		}
		if ipTrusted(ip) {
			continue
		}
		return ip
	}
	return ""
}

func ipTrusted(ip string) bool {
	parsed := net.ParseIP(strings.TrimSpace(ip))
	if parsed == nil {
		return false
	}
	for _, n := range trustedProxyNets() {
		if n.Contains(parsed) {
			return true
		}
	}
	return false
}

func trustedProxyNets() []*net.IPNet {
	raw := os.Getenv("STORAGE_TRUSTED_PROXIES")
	var out []*net.IPNet
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.Contains(part, "/") {
			parsed := net.ParseIP(part)
			if parsed == nil {
				continue
			}
			bits := 32
			if parsed.To4() == nil {
				bits = 128
			}
			part = part + "/" + strconv.Itoa(bits)
		}
		_, network, err := net.ParseCIDR(part)
		if err != nil {
			continue
		}
		out = append(out, network)
	}
	return out
}

func tenantHint(r *http.Request) string {
	if h := strings.TrimSpace(r.Header.Get("X-DataSafe-Tenant")); h != "" {
		return h
	}
	return strings.TrimSpace(r.URL.Query().Get("tenant"))
}
