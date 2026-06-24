package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/federation"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/policy"
	"github.com/DirektorBani/datasafe/internal/storage"
)

type Service struct {
	Backend  *storage.FSBackend
	Meta     metadata.MetadataStore
	Signer   *auth.Signer
	Region   string
	OwnerKey string
	SSE      *storage.SSECipher
	// FederationPeers lists registered peer clusters for cross-node GetObject.
	FederationPeers func() ([]metadata.FederationCluster, error)
	// OnObjectEvent is called after successful put/delete/copy (event: put|delete|copy).
	OnObjectEvent func(event, bucket, key string)
	// OnBucketNotification delivers bucket-level S3 notification webhooks.
	OnBucketNotification func(webhookURL, event, bucket, key string, size int64)
}

func NewService(backend *storage.FSBackend, meta metadata.MetadataStore, signer *auth.Signer, region, ownerKey string) *Service {
	return &Service{
		Backend:  backend,
		Meta:     meta,
		Signer:   signer,
		Region:   region,
		OwnerKey: ownerKey,
	}
}

// SetSSE enables SSE-S3 encryption for object payloads.
func (s *Service) SetSSE(c *storage.SSECipher) {
	s.SSE = c
}

func (s *Service) IsPublicReadBucket(logicalBucket string) bool {
	buckets, err := s.Meta.ListBuckets()
	if err != nil {
		return false
	}
	for _, rec := range buckets {
		if rec.Name == logicalBucket && rec.Visibility == "public-read" {
			return true
		}
	}
	return false
}

func (s *Service) PublicReadStorageKey(logicalBucket string) (string, error) {
	return s.publicReadStorageKey(logicalBucket)
}

func (s *Service) publicReadStorageKey(logicalBucket string) (string, error) {
	buckets, err := s.Meta.ListBuckets()
	if err != nil {
		return "", err
	}
	for _, rec := range buckets {
		if rec.Name == logicalBucket && rec.Visibility == "public-read" {
			return rec.EffectiveStorageKey(), nil
		}
	}
	return "", metadata.ErrNotFound
}

func (s *Service) Authorize(logicalBucket, key, action, principal string) bool {
	if principal == "" {
		return false
	}
	if principal == s.OwnerKey {
		return true
	}
	if logicalBucket == "" {
		return true
	}
	_, rec, err := s.ResolveBucketKey(principal, logicalBucket)
	if err != nil {
		return false
	}
	if ak, err := s.Meta.GetAccessKey(principal); err == nil {
		if !s.bucketVisibleToAccessKey(ak, rec) {
			return false
		}
		if auth.IsS3WriteAction(action) {
			if key != "" {
				return s.objectWritableByAccessKey(ak, rec, key)
			}
			return s.bucketWritableByAccessKey(ak, rec)
		}
		if key != "" {
			return s.objectVisibleToAccessKey(ak, rec, key)
		}
		return true
	}
	if rec.Owner == principal {
		return true
	}
	resource := policy.BucketARN(logicalBucket)
	if key != "" {
		resource = policy.ObjectARN(logicalBucket, key)
	}
	return policy.Evaluate(rec.Policy, action, resource, principal)
}

func (s *Service) userTenantMemberships(userID string) []auth.TenantMembership {
	if userID == "" {
		return nil
	}
	recs, _ := s.Meta.ListUserTenants(userID)
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

func (s *Service) bucketVisibleToAccessKey(ak metadata.AccessKeyRecord, rec metadata.BucketRecord) bool {
	role, userID, username, teamIDs := s.accessKeyIdentity(ak)
	return s.resolveBucketAccess(role, userID, username, teamIDs, rec, false)
}

func (s *Service) bucketWritableByAccessKey(ak metadata.AccessKeyRecord, rec metadata.BucketRecord) bool {
	role, userID, username, teamIDs := s.accessKeyIdentity(ak)
	return s.resolveBucketAccess(role, userID, username, teamIDs, rec, true)
}

func (s *Service) ListBucketsForAccessKey(ctx context.Context, accessKey string) ([]metadata.BucketRecord, error) {
	if accessKey == s.OwnerKey {
		return s.Meta.ListBuckets()
	}
	ak, err := s.Meta.GetAccessKey(accessKey)
	if err != nil {
		return nil, err
	}
	userID := ak.OwnerID
	username := ak.Owner
	role := metadata.RoleUser
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
	filter := metadata.BucketListFilter{Unfiltered: auth.CanSeeAllBuckets(role)}
	if !filter.Unfiltered {
		tenants := s.userTenantMemberships(userID)
		filter.TeamIDs = s.userTeamIDs(userID)
		filter.TenantIDs = tenantIDsFromMemberships(tenants)
		filter.TenantAdminIDs = s.tenantAdminIDs(tenants)
		filter.GroupBucketKeys = s.groupBucketKeysForUser(userID)
		filter.TenantsWithGroups = s.tenantsWithGroupsForUser(tenants)
		filter.GrantBucketKeys = s.grantBucketKeysForUser(userID)
		filter.UserID = userID
		filter.Username = username
	}
	return s.Meta.ListBucketsFiltered(filter)
}

func (s *Service) bucketScopeForPrincipal(principal string) metadata.BucketScope {
	_, ownerID, _, tenantID := s.resolveBucketOwner(principal)
	if metadata.IsTenantScoped(tenantID) {
		return metadata.BucketScope{Kind: metadata.ScopeTenant, TenantID: tenantID}
	}
	if ownerID != "" {
		members, _ := s.Meta.ListUserTenants(ownerID)
		return metadata.BucketScopeForUser(tenantID, ownerID, members)
	}
	return metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: ownerID}
}

// ResolveBucketKey maps a logical bucket name to storage key for the given principal.
func (s *Service) ResolveBucketKey(principal, logicalName string) (string, metadata.BucketRecord, error) {
	if principal == s.OwnerKey {
		if rec, err := s.Meta.GetBucketByKey(logicalName); err == nil {
			return rec.EffectiveStorageKey(), rec, nil
		}
		if buckets, err := s.Meta.ListBuckets(); err == nil {
			for _, b := range buckets {
				if b.Name == logicalName {
					return b.EffectiveStorageKey(), b, nil
				}
			}
		}
	}
	scope := s.bucketScopeForPrincipal(principal)
	rec, err := s.Meta.ResolveBucket(scope, logicalName)
	if err != nil {
		_, ownerID, _, _ := s.resolveBucketOwner(principal)
		if scope.Kind != metadata.ScopeOwner && ownerID != "" {
			ownerScope := metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: ownerID}
			if ownerRec, ownerErr := s.Meta.ResolveBucket(ownerScope, logicalName); ownerErr == nil {
				rec = ownerRec
				err = nil
			}
		}
		if err != nil {
			rec, err = s.Meta.GetBucket(logicalName)
			if err != nil {
				return "", rec, err
			}
		}
	}
	return rec.EffectiveStorageKey(), rec, nil
}

// normalizeBucketKey accepts either a storage key or logical bucket name.
func (s *Service) normalizeBucketKey(bucket string) string {
	if rec, err := s.Meta.GetBucketByKey(bucket); err == nil {
		return rec.EffectiveStorageKey()
	}
	if all, err := s.Meta.ListBuckets(); err == nil {
		for _, b := range all {
			if b.Name == bucket {
				return b.EffectiveStorageKey()
			}
		}
	}
	return bucket
}

func (s *Service) resolveBucketOwner(principal string) (owner, ownerID, teamID, tenantID string) {
	owner = principal
	if u, err := s.Meta.GetUserByUsername(principal); err == nil {
		owner = u.Username
		ownerID = u.ID
		teamID = u.TeamID
		tenantID = u.TenantID
		return
	}
	ak, err := s.Meta.GetAccessKey(principal)
	if err != nil {
		return
	}
	ownerID = ak.OwnerID
	owner = ak.Owner
	if ownerID != "" {
		if u, uerr := s.Meta.GetUser(ownerID); uerr == nil {
			if owner == "" {
				owner = u.Username
			}
			teamID = u.TeamID
			tenantID = u.TenantID
		}
		return
	}
	if owner != "" {
		if u, uerr := s.Meta.GetUserByUsername(owner); uerr == nil {
			ownerID = u.ID
			teamID = u.TeamID
			tenantID = u.TenantID
		}
	}
	return
}

func (s *Service) CreateBucket(ctx context.Context, name, owner string) error {
	ownerName, ownerID, teamID, tenantID := s.resolveBucketOwner(owner)
	scope := s.bucketScopeForPrincipal(owner)
	if scope.Kind == metadata.ScopeOwner && ownerID == "" {
		scope.OwnerID = ownerName
	}
	storageKey := metadata.MakeStorageKey(scope, name)
	backendErr := s.Backend.CreateBucket(storageKey)
	if backendErr != nil && backendErr != storage.ErrBucketExists {
		return backendErr
	}
	rec := metadata.BucketRecord{
		StorageKey: storageKey,
		Name:       name,
		CreatedAt:  time.Now().UTC(),
		Owner:      ownerName,
		OwnerID:    ownerID,
		TeamID:     teamID,
	}
	if scope.Kind == metadata.ScopeTenant {
		rec.TenantID = scope.TenantID
	} else if tenantID != "" {
		rec.TenantID = tenantID
	}
	if rec.TenantID == "" {
		rec.TenantID = metadata.DefaultTenantID
	}
	if err := s.Meta.PutBucket(rec); err != nil {
		if backendErr == nil {
			_ = s.Backend.DeleteBucket(storageKey)
		}
		return err
	}
	return nil
}

func (s *Service) DeleteBucket(ctx context.Context, storageKey string) error {
	storageKey = s.normalizeBucketKey(storageKey)
	if err := s.Meta.DeleteBucket(storageKey); err != nil {
		return err
	}
	return s.Backend.DeleteBucket(storageKey)
}

func (s *Service) ListBuckets(ctx context.Context) ([]metadata.BucketRecord, error) {
	return s.Meta.ListBuckets()
}

func (s *Service) bucketVersioning(bucket string) bool {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return false
	}
	return rec.Versioning
}

func (s *Service) bucketVersioningSuspended(bucket string) bool {
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return false
	}
	return rec.VersioningSuspended
}

func (s *Service) versioningActive(bucket string) bool {
	return s.bucketVersioning(bucket) && !s.bucketVersioningSuspended(bucket)
}

func (s *Service) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string, userMeta map[string]string) (metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	bucketRec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	if err := s.checkQuota(bucketRec, bucket, key, size); err != nil {
		return metadata.ObjectRecord{}, err
	}
	if s.SSE != nil {
		encReader, err := s.SSE.EncryptReader(r)
		if err != nil {
			return metadata.ObjectRecord{}, err
		}
		r = encReader
	}
	versionID := newVersionID()
	var etag string
	versionedPut := s.versioningActive(bucket)
	suspended := s.bucketVersioning(bucket) && s.bucketVersioningSuspended(bucket)
	if versionedPut {
		etag, err = s.Backend.PutObjectVersion(ctx, bucket, key, versionID, r, size, contentType)
	} else if suspended {
		existing, err := s.Meta.GetObject(bucket, key)
		if err == nil && existing.VersionID != "" && !existing.IsDeleteMarker {
			versionID = existing.VersionID
			etag, err = s.Backend.PutObjectVersion(ctx, bucket, key, versionID, r, size, contentType)
		} else {
			s.purgeObjectData(ctx, bucket, key)
			etag, err = s.Backend.PutObject(ctx, bucket, key, r, size, contentType)
			versionID = ""
		}
	} else {
		s.purgeObjectData(ctx, bucket, key)
		etag, err = s.Backend.PutObject(ctx, bucket, key, r, size, contentType)
		versionID = ""
	}
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	var info storage.ObjectInfo
	if versionedPut || (suspended && versionID != "") {
		info, err = s.Backend.StatObjectVersion(ctx, bucket, key, versionID)
	} else {
		info, err = s.Backend.StatObject(ctx, bucket, key)
	}
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	rec := metadata.ObjectRecord{
		Bucket:       bucket,
		Key:          key,
		Size:         info.Size,
		ETag:         etag,
		ContentType:  contentType,
		VersionID:    versionID,
		LastModified: info.LastModified,
		Metadata:     userMeta,
		CreatedAt:    info.LastModified,
	}
	if bucketRec.StorageClass != "" {
		rec.StorageClass = bucketRec.StorageClass
	}
	if bucketRec.ObjectLock && bucketRec.RetentionDays > 0 {
		until := info.LastModified.Add(time.Duration(bucketRec.RetentionDays) * 24 * time.Hour)
		rec.RetentionUntil = &until
	}
	if existing, err := s.Meta.GetObject(bucket, key); err == nil && !existing.IsDeleteMarker {
		if !existing.CreatedAt.IsZero() {
			rec.CreatedAt = existing.CreatedAt
		}
	}
	if versionedPut {
		err = s.Meta.PutObjectVersioned(rec)
	} else if suspended && versionID != "" {
		err = s.Meta.PutObjectVersioned(rec)
	} else {
		err = s.Meta.PutObject(rec)
	}
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	if s.OnObjectEvent != nil {
		s.OnObjectEvent(metadata.ReplEventPut, bucket, key)
	}
	return rec, nil
}

func (s *Service) purgeObjectData(ctx context.Context, bucket, key string) {
	ids, _ := s.Meta.ListObjectVersionIDs(bucket, key)
	for _, id := range ids {
		_ = s.Backend.DeleteObjectVersion(ctx, bucket, key, id)
	}
	_ = s.Backend.DeleteAllObjectVersions(ctx, bucket, key)
}

func (s *Service) GetObject(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	var rec metadata.ObjectRecord
	var err error
	if versionID != "" {
		rec, err = s.Meta.GetObjectVersion(bucket, key, versionID)
	} else {
		rec, err = s.Meta.GetObject(bucket, key)
	}
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) && versionID == "" {
			if rc, fedRec, fedErr := s.getObjectFromFederation(ctx, bucket, key); fedErr == nil {
				return rc, fedRec, nil
			}
		}
		return nil, metadata.ObjectRecord{}, err
	}
	vid := rec.VersionID
	rc, _, err := s.Backend.GetObjectVersion(ctx, bucket, key, vid)
	if err != nil {
		return nil, metadata.ObjectRecord{}, err
	}
	if s.SSE != nil {
		dec, err := s.SSE.DecryptReader(rc)
		rc.Close()
		if err != nil {
			return nil, metadata.ObjectRecord{}, err
		}
		return io.NopCloser(dec), rec, nil
	}
	return rc, rec, nil
}

func (s *Service) getObjectFromFederation(ctx context.Context, storageBucket, key string) (io.ReadCloser, metadata.ObjectRecord, error) {
	if s.FederationPeers == nil {
		return nil, metadata.ObjectRecord{}, metadata.ErrNotFound
	}
	peers, err := s.FederationPeers()
	if err != nil || len(peers) == 0 {
		return nil, metadata.ObjectRecord{}, metadata.ErrNotFound
	}
	logicalBucket := storageBucket
	if rec, err := s.Meta.GetBucketByKey(storageBucket); err == nil {
		logicalBucket = rec.Name
	}
	for _, peer := range peers {
		if !peerCanRead(peer) {
			continue
		}
		data, ct, err := federation.ProxyGetObject(ctx, peer.Endpoint, logicalBucket, key)
		if err != nil {
			continue
		}
		rec := metadata.ObjectRecord{
			Bucket:       storageBucket,
			Key:          key,
			Size:         int64(len(data)),
			ContentType:  ct,
			LastModified: time.Now().UTC(),
		}
		return io.NopCloser(bytes.NewReader(data)), rec, nil
	}
	return nil, metadata.ObjectRecord{}, metadata.ErrNotFound
}

func peerCanRead(peer metadata.FederationCluster) bool {
	if len(peer.Capabilities) == 0 {
		return true
	}
	for _, c := range peer.Capabilities {
		if c == "read" || c == "list" {
			return true
		}
	}
	return false
}

func (s *Service) HeadObject(ctx context.Context, bucket, key, versionID string) (metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	if versionID != "" {
		return s.Meta.GetObjectVersion(bucket, key, versionID)
	}
	return s.Meta.GetObject(bucket, key)
}

func (s *Service) GetObjectTags(ctx context.Context, bucket, key, versionID string) (map[string]string, error) {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.HeadObject(ctx, bucket, key, versionID)
	if err != nil {
		return nil, err
	}
	if rec.Tags == nil {
		return map[string]string{}, nil
	}
	return rec.Tags, nil
}

func (s *Service) SetObjectTags(ctx context.Context, bucket, key, versionID string, tags map[string]string) error {
	bucket = s.normalizeBucketKey(bucket)
	return s.Meta.SetObjectTags(bucket, key, versionID, tags)
}

func (s *Service) checkDeleteAllowed(bucket, key, versionID string) error {
	rec, err := s.Meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	if rec.LegalHold {
		return metadata.ErrLegalHold
	}
	if rec.RetentionUntil != nil && time.Now().UTC().Before(*rec.RetentionUntil) {
		return metadata.ErrRetentionLocked
	}
	bucketRec, err := s.Meta.GetBucketByKey(bucket)
	if err == nil && bucketRec.ObjectLock && bucketRec.RetentionDays > 0 {
		base := rec.CreatedAt
		if base.IsZero() {
			base = rec.LastModified
		}
		until := base.Add(time.Duration(bucketRec.RetentionDays) * 24 * time.Hour)
		if time.Now().UTC().Before(until) {
			return metadata.ErrRetentionLocked
		}
	}
	return nil
}

func (s *Service) DeleteObject(ctx context.Context, bucket, key, versionID string) error {
	bucket = s.normalizeBucketKey(bucket)
	if err := s.checkDeleteAllowed(bucket, key, versionID); err != nil {
		return err
	}
	versioning := s.bucketVersioning(bucket)
	suspended := s.bucketVersioningSuspended(bucket)
	if versioning && !suspended && versionID == "" {
		markerID := newVersionID()
		rec := metadata.ObjectRecord{
			Bucket:         bucket,
			Key:            key,
			Size:           0,
			VersionID:      markerID,
			IsDeleteMarker: true,
			LastModified:   time.Now().UTC(),
		}
		if err := s.Meta.PutObjectVersioned(rec); err != nil {
			return err
		}
		if s.OnObjectEvent != nil {
			s.OnObjectEvent(metadata.ReplEventDelete, bucket, key)
		}
		return nil
	}
	rec, err := s.Meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	if rec.IsDeleteMarker {
		return metadata.ErrNotFound
	}
	vid := rec.VersionID
	if err := s.Meta.DeleteObjectVersion(bucket, key, versionID, versioning); err != nil {
		return err
	}
	if err := s.Backend.DeleteObjectVersion(ctx, bucket, key, vid); err != nil && err != storage.ErrNotFound {
		return err
	}
	if !versioning {
		_ = s.Backend.DeleteAllObjectVersions(ctx, bucket, key)
	}
	if s.OnObjectEvent != nil {
		s.OnObjectEvent(metadata.ReplEventDelete, bucket, key)
	}
	return nil
}

func (s *Service) ListObjectVersions(ctx context.Context, bucket, prefix string, maxKeys int) ([]metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	if _, err := s.Meta.GetBucketByKey(bucket); err != nil {
		return nil, err
	}
	return s.Meta.ListObjectVersions(bucket, prefix, maxKeys)
}

func (s *Service) ListObjects(ctx context.Context, bucket, prefix string, maxKeys int) ([]metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	logicalBucket := bucket
	if rec, err := s.Meta.GetBucketByKey(bucket); err == nil {
		logicalBucket = rec.Name
	} else if _, err := s.Meta.GetBucket(bucket); err != nil {
		return nil, err
	}
	objs, err := s.Meta.ListObjects(bucket, prefix, maxKeys)
	if err != nil {
		return nil, err
	}
	return s.mergeFederationList(ctx, logicalBucket, prefix, maxKeys, objs)
}

func (s *Service) mergeFederationList(ctx context.Context, logicalBucket, prefix string, maxKeys int, local []metadata.ObjectRecord) ([]metadata.ObjectRecord, error) {
	if s.FederationPeers == nil {
		return local, nil
	}
	peers, err := s.FederationPeers()
	if err != nil || len(peers) == 0 {
		return local, nil
	}
	var federated []federation.FederatedObject
	for _, peer := range peers {
		if !peerCanRead(peer) {
			continue
		}
		items, err := federation.ProxyListObjects(ctx, peer, logicalBucket, prefix, maxKeys)
		if err != nil {
			continue
		}
		federated = append(federated, items...)
	}
	merged := federation.MergeFederatedObjects(local, federated)
	if maxKeys > 0 && len(merged) > maxKeys {
		merged = merged[:maxKeys]
	}
	return merged, nil
}

func (s *Service) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (metadata.ObjectRecord, error) {
	srcBucket = s.normalizeBucketKey(srcBucket)
	dstBucket = s.normalizeBucketKey(dstBucket)
	src, err := s.Meta.GetObject(srcBucket, srcKey)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	srcVID := src.VersionID
	rc, _, err := s.Backend.GetObjectVersion(ctx, srcBucket, srcKey, srcVID)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	defer rc.Close()
	rec, err := s.PutObject(ctx, dstBucket, dstKey, rc, src.Size, src.ContentType, src.Metadata)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	if len(src.Tags) > 0 {
		_ = s.Meta.SetObjectTags(dstBucket, dstKey, rec.VersionID, src.Tags)
		rec.Tags = src.Tags
	}
	if src.StorageClass != "" && rec.StorageClass == "" {
		rec.StorageClass = src.StorageClass
	}
	return rec, nil
}

func (s *Service) CreateMultipartUpload(ctx context.Context, bucket, key string) (string, error) {
	bucket = s.normalizeBucketKey(bucket)
	if _, err := s.Meta.GetBucketByKey(bucket); err != nil {
		return "", err
	}
	uploadID := newUploadID()
	if err := s.Backend.CreateMultipartUpload(ctx, bucket, key, uploadID); err != nil {
		return "", err
	}
	rec := metadata.MultipartRecord{
		UploadID:  uploadID,
		Bucket:    bucket,
		Key:       key,
		Initiated: time.Now().UTC(),
	}
	if err := s.Meta.PutMultipart(rec); err != nil {
		return "", err
	}
	return uploadID, nil
}

func (s *Service) UploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, r io.Reader, size int64) (string, error) {
	bucket = s.normalizeBucketKey(bucket)
	mp, err := s.Meta.GetMultipart(uploadID)
	if err != nil {
		return "", err
	}
	if mp.Bucket != bucket || mp.Key != key {
		return "", metadata.ErrNotFound
	}
	return s.Backend.UploadPart(ctx, bucket, key, uploadID, partNumber, r, size)
}

func (s *Service) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []storage.PartInfo) (metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	mp, err := s.Meta.GetMultipart(uploadID)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	if mp.Bucket != bucket || mp.Key != key {
		return metadata.ObjectRecord{}, metadata.ErrNotFound
	}
	etag, err := s.Backend.CompleteMultipartUpload(ctx, bucket, key, uploadID, parts)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	info, err := s.Backend.StatObject(ctx, bucket, key)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	bucketRec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	if err := s.checkQuota(bucketRec, bucket, key, info.Size); err != nil {
		return metadata.ObjectRecord{}, err
	}
	versionID := newVersionID()
	rec := metadata.ObjectRecord{
		Bucket:       bucket,
		Key:          key,
		Size:         info.Size,
		ETag:         etag,
		VersionID:    versionID,
		LastModified: info.LastModified,
	}
	if bucketRec.Versioning {
		// Move completed object into versioned path.
		rc, _, err := s.Backend.GetObject(ctx, bucket, key)
		if err != nil {
			return metadata.ObjectRecord{}, err
		}
		_, err = s.Backend.PutObjectVersion(ctx, bucket, key, versionID, rc, info.Size, "")
		rc.Close()
		if err != nil {
			return metadata.ObjectRecord{}, err
		}
		_ = s.Backend.DeleteObject(ctx, bucket, key)
		info, _ = s.Backend.StatObjectVersion(ctx, bucket, key, versionID)
		rec.LastModified = info.LastModified
		err = s.Meta.PutObjectVersioned(rec)
	} else {
		rec.VersionID = ""
		err = s.Meta.PutObject(rec)
	}
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	_ = s.Meta.DeleteMultipart(uploadID)
	return rec, nil
}

func (s *Service) AbortMultipartUpload(ctx context.Context, bucket, key, uploadID string) error {
	bucket = s.normalizeBucketKey(bucket)
	mp, err := s.Meta.GetMultipart(uploadID)
	if err != nil {
		return err
	}
	if mp.Bucket != bucket || mp.Key != key {
		return metadata.ErrNotFound
	}
	if err := s.Backend.AbortMultipartUpload(ctx, bucket, key, uploadID); err != nil {
		return err
	}
	return s.Meta.DeleteMultipart(uploadID)
}

// PruneScheduledDeletesOnce deletes objects whose scheduled deletion time has passed.
func (s *Service) PruneScheduledDeletesOnce(ctx context.Context) []metadata.ObjectRecord {
	buckets, err := s.Meta.ListBuckets()
	if err != nil {
		return nil
	}
	now := time.Now().UTC()
	var deleted []metadata.ObjectRecord
	for _, b := range buckets {
		objs, err := s.Meta.ListObjects(b.Name, "", 0)
		if err != nil {
			continue
		}
		for _, obj := range objs {
			if obj.ScheduledDeleteAt == nil || obj.ScheduledDeleteAt.After(now) {
				continue
			}
				if err := s.DeleteObject(ctx, b.Name, obj.Key, ""); err != nil {
				continue
			}
			deleted = append(deleted, obj)
		}
	}
	return deleted
}

func (s *Service) PruneExpiredOnce(ctx context.Context) {
	s.pruneExpired(ctx)
}

func (s *Service) RunLifecycle(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pruneExpired(ctx)
		}
	}
}

func (s *Service) pruneExpired(ctx context.Context) {
	buckets, err := s.Meta.ListBuckets()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for _, b := range buckets {
		for _, rule := range b.LifecycleRules {
			if !rule.Enabled {
				continue
			}
			action := rule.Action
			if action == "" {
				action = metadata.LifecycleExpire
			}
			switch action {
			case metadata.LifecycleAbortMultipart:
				if rule.ExpirationDays <= 0 {
					continue
				}
				cutoff := now.Add(-time.Duration(rule.ExpirationDays) * 24 * time.Hour)
				mps, err := s.Meta.ListMultipart(b.Name)
				if err != nil {
					continue
				}
				for _, mp := range mps {
					if mp.Initiated.Before(cutoff) {
						_ = s.AbortMultipartUpload(ctx, mp.Bucket, mp.Key, mp.UploadID)
					}
				}
			case metadata.LifecycleExpireNoncurrent:
				if rule.ExpirationDays <= 0 {
					continue
				}
				cutoff := now.Add(-time.Duration(rule.ExpirationDays) * 24 * time.Hour)
				versions, err := s.Meta.ListObjectVersions(b.Name, rule.Prefix, 0)
				if err != nil {
					continue
				}
				latestByKey := map[string]string{}
				for _, v := range versions {
					latest, err := s.Meta.GetObject(b.Name, v.Key)
					if err == nil && latest.VersionID == v.VersionID {
						latestByKey[v.Key] = v.VersionID
					}
				}
				for _, v := range versions {
					if v.VersionID == "" || v.VersionID == latestByKey[v.Key] {
						continue
					}
					if v.LastModified.Before(cutoff) {
						_ = s.DeleteObject(ctx, b.Name, v.Key, v.VersionID)
					}
				}
			default:
				if rule.ExpirationDays <= 0 {
					continue
				}
				objs, err := s.Meta.ListObjects(b.Name, rule.Prefix, 0)
				if err != nil {
					continue
				}
				cutoff := now.Add(-time.Duration(rule.ExpirationDays) * 24 * time.Hour)
				for _, obj := range objs {
					if obj.LastModified.Before(cutoff) {
						_ = s.DeleteObject(ctx, b.Name, obj.Key, "")
					}
				}
			}
		}
	}
}

func (s *Service) GetBucketVersioning(bucket string) (string, error) {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return "", err
	}
	if rec.Versioning && !rec.VersioningSuspended {
		return "Enabled", nil
	}
	if rec.Versioning {
		return "Suspended", nil
	}
	return "Suspended", nil
}

func (s *Service) SetBucketVersioning(bucket string, status string) error {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return err
	}
	switch strings.ToLower(status) {
	case "enabled":
		rec.Versioning = true
		rec.VersioningSuspended = false
	case "suspended":
		rec.Versioning = true
		rec.VersioningSuspended = true
	default:
		rec.Versioning = false
		rec.VersioningSuspended = false
	}
	return s.Meta.UpdateBucket(rec)
}

func (s *Service) GetBucketLifecycle(bucket string) ([]metadata.LifecycleRule, error) {
	bucket = s.normalizeBucketKey(bucket)
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return nil, err
	}
	return rec.LifecycleRules, nil
}

func (s *Service) SetBucketLifecycle(bucket string, rules []metadata.LifecycleRule) error {
	bucket = s.normalizeBucketKey(bucket)
	return s.Meta.SetBucketLifecycle(bucket, rules)
}

func (s *Service) EnsureTrashBucket(ctx context.Context) error {
	if _, err := s.Meta.GetBucket(metadata.TrashBucketName); err == nil {
		return nil
	}
	return s.CreateBucket(ctx, metadata.TrashBucketName, "system")
}

func (s *Service) MoveToTrash(ctx context.Context, bucket, key, versionID, deletedBy string) (metadata.TrashRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	if err := s.EnsureTrashBucket(ctx); err != nil {
		return metadata.TrashRecord{}, err
	}
	src, err := s.Meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return metadata.TrashRecord{}, err
	}
	if src.IsDeleteMarker {
		return metadata.TrashRecord{}, metadata.ErrNotFound
	}
	srcVID := src.VersionID
	rc, _, err := s.Backend.GetObjectVersion(ctx, bucket, key, srcVID)
	if err != nil {
		return metadata.TrashRecord{}, err
	}
	body, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return metadata.TrashRecord{}, err
	}
	trashKey := bucket + "/" + key + "/" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	if srcVID != "" {
		trashKey += "/" + srcVID
	}
	_, err = s.PutObject(ctx, metadata.TrashBucketName, trashKey, bytes.NewReader(body), src.Size, src.ContentType, src.Metadata)
	if err != nil {
		return metadata.TrashRecord{}, err
	}
	tr := metadata.TrashRecord{
		ID:             newVersionID(),
		OriginalBucket: bucket,
		OriginalKey:    key,
		TrashKey:       trashKey,
		Size:           src.Size,
		VersionID:      srcVID,
		DeletedBy:      deletedBy,
		DeletedAt:      time.Now().UTC(),
	}
	if err := s.Meta.PutTrash(tr); err != nil {
		return metadata.TrashRecord{}, err
	}
	if err := s.DeleteObject(ctx, bucket, key, versionID); err != nil {
		return metadata.TrashRecord{}, err
	}
	return tr, nil
}

func (s *Service) RestoreFromTrash(ctx context.Context, trashID string) (metadata.ObjectRecord, error) {
	tr, err := s.Meta.GetTrash(trashID)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	rc, _, err := s.Backend.GetObject(ctx, metadata.TrashBucketName, tr.TrashKey)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	defer rc.Close()
	rec, err := s.PutObject(ctx, tr.OriginalBucket, tr.OriginalKey, rc, tr.Size, "", nil)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	_ = s.Backend.DeleteObject(ctx, metadata.TrashBucketName, tr.TrashKey)
	_ = s.Meta.DeleteTrash(trashID)
	return rec, nil
}

func (s *Service) PurgeTrashItem(ctx context.Context, trashID string) error {
	tr, err := s.Meta.GetTrash(trashID)
	if err != nil {
		return err
	}
	_ = s.Backend.DeleteObject(ctx, metadata.TrashBucketName, tr.TrashKey)
	return s.Meta.DeleteTrash(trashID)
}

func (s *Service) PruneTrashOnce(ctx context.Context, retentionDays int) []metadata.TrashRecord {
	if retentionDays <= 0 {
		return nil
	}
	items, err := s.Meta.ListTrash("")
	if err != nil {
		return nil
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var purged []metadata.TrashRecord
	for _, tr := range items {
		if tr.DeletedAt.After(cutoff) {
			continue
		}
		if err := s.PurgeTrashItem(ctx, tr.ID); err != nil {
			continue
		}
		purged = append(purged, tr)
	}
	return purged
}

func (s *Service) RestoreObjectVersion(ctx context.Context, bucket, key, versionID string) (metadata.ObjectRecord, error) {
	bucket = s.normalizeBucketKey(bucket)
	rc, rec, err := s.GetObject(ctx, bucket, key, versionID)
	if err != nil {
		return metadata.ObjectRecord{}, err
	}
	defer rc.Close()
	return s.PutObject(ctx, bucket, key, rc, rec.Size, rec.ContentType, rec.Metadata)
}

func (s *Service) checkQuota(bucketRec metadata.BucketRecord, bucket, key string, newSize int64) error {
	var existingSize int64
	isNewKey := true
	if existing, err := s.Meta.GetObject(bucket, key); err == nil && !existing.IsDeleteMarker {
		isNewKey = false
		existingSize = existing.Size
	}
	netSize := newSize - existingSize
	if netSize < 0 {
		netSize = 0
	}

	if bucketRec.MaxObjects > 0 && isNewKey {
		count, err := s.Meta.BucketObjectCount(bucketRec.EffectiveStorageKey())
		if err != nil {
			return err
		}
		if int64(count) >= bucketRec.MaxObjects {
			return metadata.ErrQuotaExceeded
		}
	}
	if bucketRec.MaxSizeBytes > 0 {
		total, err := s.Meta.BucketTotalSize(bucketRec.EffectiveStorageKey())
		if err != nil {
			return err
		}
		if total+netSize > bucketRec.MaxSizeBytes {
			return metadata.ErrQuotaExceeded
		}
	}
	owner := bucketRec.Owner
	if owner == "" {
		return nil
	}
	user, err := s.Meta.GetUserByUsername(owner)
	if err != nil {
		return nil
	}
	if user.MaxObjects > 0 && isNewKey {
		objCount, _, err := s.Meta.OwnerUsage(owner)
		if err != nil {
			return err
		}
		if int64(objCount) >= user.MaxObjects {
			return metadata.ErrQuotaExceeded
		}
	}
	if user.MaxSizeBytes > 0 {
		_, usedBytes, err := s.Meta.OwnerUsage(owner)
		if err != nil {
			return err
		}
		if usedBytes+netSize > user.MaxSizeBytes {
			return metadata.ErrQuotaExceeded
		}
	}
	return nil
}

func newVersionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func newUploadID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func ParseRange(header string, size int64) (start, end int64, ok bool) {
	if header == "" {
		return 0, size - 1, true
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, false
	}
	spec := strings.TrimPrefix(header, "bytes=")
	if strings.HasPrefix(spec, "-") {
		n, err := strconv.ParseInt(strings.TrimPrefix(spec, "-"), 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, false
		}
		if n > size {
			n = size
		}
		return size - n, size - 1, true
	}
	parts := strings.SplitN(spec, "-", 2)
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	if parts[1] == "" {
		return start, size - 1, true
	}
	end, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err2 != nil || end < start {
		return 0, 0, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true
}

func ContentRange(start, end, total int64) string {
	return fmt.Sprintf("bytes %d-%d/%d", start, end, total)
}
