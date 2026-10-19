package s3

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/storage"
)

// ActivityHook records an Admin Activity row from the S3 path (optional).
// user is a display name (access-key owner username when known).
type ActivityHook func(user, ip, action, resourceType, resourceName string)

// PresignDecryptHook records a presigned GET that decrypted or refused a sealed object.
// result is "ok", "key_destroyed", or "kms_unavailable".
type PresignDecryptHook func(ctx context.Context, user, ip, bucket, key, keyID, tenant, result string, keyVersion int)

// PublicReadHook records an anonymous GET (plaintext, SSE-S3, or a KMS refusal).
// result is "ok", "public_blocked", or "kms_public".
type PublicReadHook func(ctx context.Context, ip, bucket, key, result, keyID string, keyVersion int)

// PresignBlockedHook records a presigned GET refused because sharing is off.
type PresignBlockedHook func(ctx context.Context, accessKey, ip, bucket, reason string)

type Handler struct {
	Svc              *Service
	OnActivity       ActivityHook
	OnPresignDecrypt PresignDecryptHook
	OnPublicRead     PublicReadHook
	OnPresignBlocked PresignBlockedHook
}

func NewHandler(svc *Service) *Handler {
	return &Handler{Svc: svc}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" || r.URL.Path == "" {
		switch r.Method {
		case http.MethodGet:
			h.listBuckets(w, r)
		default:
			writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		}
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		writeError(w, r, http.StatusBadRequest, "InvalidRequest", "invalid path")
		return
	}
	parts := strings.SplitN(path, "/", 2)
	bucket := parts[0]
	key := ""
	if len(parts) == 2 {
		key = parts[1]
	}

	q := r.URL.Query()
	if key == "" {
		switch r.Method {
		case http.MethodPut:
			if q.Has("encryption") {
				h.putBucketEncryption(w, r, bucket)
				return
			}
			if q.Has("policy") {
				h.putBucketPolicy(w, r, bucket)
				return
			}
			if q.Has("versioning") {
				h.putBucketVersioning(w, r, bucket)
				return
			}
			if q.Has("lifecycle") {
				h.putBucketLifecycle(w, r, bucket)
				return
			}
			if q.Has("object-lock") {
				h.putBucketObjectLock(w, r, bucket)
				return
			}
			if q.Has("notification") {
				h.putBucketNotification(w, r, bucket)
				return
			}
			h.createBucket(w, r, bucket)
		case http.MethodDelete:
			if q.Has("encryption") {
				h.deleteBucketEncryption(w, r, bucket)
				return
			}
			if q.Has("policy") {
				h.deleteBucketPolicy(w, r, bucket)
				return
			}
			if q.Has("notification") {
				h.deleteBucketNotification(w, r, bucket)
				return
			}
			h.deleteBucket(w, r, bucket)
		case http.MethodGet:
			if q.Has("encryption") {
				h.getBucketEncryption(w, r, bucket)
				return
			}
			if q.Has("versioning") {
				h.getBucketVersioning(w, r, bucket)
				return
			}
			if q.Has("lifecycle") {
				h.getBucketLifecycle(w, r, bucket)
				return
			}
			if q.Has("object-lock") {
				h.getBucketObjectLock(w, r, bucket)
				return
			}
			if q.Has("notification") {
				h.getBucketNotification(w, r, bucket)
				return
			}
			if q.Has("policy") {
				h.getBucketPolicy(w, r, bucket)
				return
			}
			if q.Has("versions") {
				h.listObjectVersions(w, r, bucket, q)
				return
			}
			if q.Has("location") {
				h.getBucketLocation(w, r, bucket)
				return
			}
			if q.Has("uploads") {
				h.listMultipartUploads(w, r, bucket, q)
				return
			}
			h.listObjects(w, r, bucket, q)
		case http.MethodHead:
			h.headBucket(w, r, bucket)
		case http.MethodPost:
			if q.Has("delete") {
				h.deleteObjects(w, r, bucket)
				return
			}
			writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		default:
			writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		}
		return
	}

	if q.Has("uploads") && r.Method == http.MethodPost {
		h.createMultipart(w, r, bucket, key)
		return
	}
	if q.Has("uploadId") {
		uploadID := q.Get("uploadId")
		switch r.Method {
		case http.MethodPut:
			h.uploadPart(w, r, bucket, key, uploadID, q)
		case http.MethodPost:
			h.completeMultipart(w, r, bucket, key, uploadID)
		case http.MethodDelete:
			h.abortMultipart(w, r, bucket, key, uploadID)
		default:
			writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
		}
		return
	}

	switch r.Method {
	case http.MethodPut:
		if q.Has("tagging") {
			h.putObjectTagging(w, r, bucket, key)
			return
		}
		if q.Has("retention") {
			h.putObjectRetention(w, r, bucket, key)
			return
		}
		if q.Has("legal-hold") {
			h.putObjectLegalHold(w, r, bucket, key)
			return
		}
		if r.Header.Get("x-amz-copy-source") != "" {
			h.copyObject(w, r, bucket, key)
		} else {
			h.putObject(w, r, bucket, key)
		}
	case http.MethodGet:
		if q.Has("tagging") {
			h.getObjectTagging(w, r, bucket, key)
			return
		}
		if q.Has("retention") {
			h.getObjectRetention(w, r, bucket, key)
			return
		}
		if q.Has("legal-hold") {
			h.getObjectLegalHold(w, r, bucket, key)
			return
		}
		h.getObject(w, r, bucket, key)
	case http.MethodHead:
		h.headObject(w, r, bucket, key)
	case http.MethodDelete:
		h.deleteObject(w, r, bucket, key)
	default:
		writeError(w, r, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (auth.Credentials, bool) {
	creds, err := h.Svc.Signer.Authenticate(r)
	if err != nil {
		switch err {
		case auth.ErrUnsignedHeaders:
			writeError(w, r, http.StatusBadRequest, "AccessDenied", "There were headers present in the request which were not signed")
		case auth.ErrMissingAuth, auth.ErrInvalidAuth, auth.ErrSignatureMismatch:
			writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
		case auth.ErrExpired:
			writeError(w, r, http.StatusForbidden, "AccessDenied", "request expired")
		default:
			writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
		}
		return auth.Credentials{}, false
	}
	return creds, true
}

func (h *Handler) authorize(w http.ResponseWriter, r *http.Request, creds auth.Credentials, bucket, key, action string) bool {
	if !h.Svc.AuthorizeRequest(bucket, key, action, creds.AccessKey, requestSourceIP(r), r.TLS != nil) {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
		return false
	}
	return true
}

func (h *Handler) storageBucket(w http.ResponseWriter, r *http.Request, creds auth.Credentials, logical string) (string, bool) {
	sk, _, err := h.Svc.ResolveBucketKey(creds.AccessKey, logical)
	if err != nil {
		writeError(w, r, http.StatusNotFound, "NoSuchBucket", "bucket not found")
		return "", false
	}
	return sk, true
}

func (h *Handler) getBucketVersioning(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:GetBucketVersioning") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	status, err := h.Svc.GetBucketVersioning(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, VersioningConfiguration{Xmlns: xmlNS, Status: status})
}

func (h *Handler) putBucketVersioning(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketVersioning") {
		return
	}
	var cfg VersioningConfiguration
	if err := xml.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	enabled := strings.EqualFold(cfg.Status, "Enabled")
	suspended := strings.EqualFold(cfg.Status, "Suspended")
	if suspended && h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	status := "Suspended"
	if enabled {
		status = "Enabled"
	} else if suspended {
		status = "Suspended"
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.SetBucketVersioning(sk, status); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getBucketLifecycle(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:GetLifecycleConfiguration") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rules, err := h.Svc.GetBucketLifecycle(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, lifecycleRulesToXML(rules))
}

func (h *Handler) putBucketLifecycle(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutLifecycleConfiguration") {
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	rules, err := parseLifecycleXML(raw)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", err.Error())
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.SetBucketLifecycle(sk, rules); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) listBuckets(w http.ResponseWriter, r *http.Request) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, "", "", "s3:ListAllMyBuckets") {
		return
	}
	buckets, err := h.Svc.ListBucketsForAccessKey(r.Context(), creds.AccessKey)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "InternalError", err.Error())
		return
	}
	resp := ListAllMyBucketsResult{
		Xmlns: xmlNS,
		Owner: Owner{ID: "datasafe", DisplayName: "Датасейф S3"},
	}
	for _, b := range buckets {
		resp.Buckets.Bucket = append(resp.Buckets.Bucket, BucketEntry{
			Name:         b.Name,
			CreationDate: formatTime(b.CreatedAt),
		})
	}
	writeXML(w, http.StatusOK, resp)
}

func (h *Handler) createBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:CreateBucket") {
		return
	}
	if err := h.Svc.CreateBucket(r.Context(), bucket, creds.AccessKey); err != nil {
		mapErr(w, r, err)
		return
	}
	if _, brec, err := h.Svc.ResolveBucketKey(creds.AccessKey, bucket); err == nil {
		changed := false
		if ak, aerr := h.Svc.Meta.GetAccessKey(creds.AccessKey); aerr == nil {
			if brec.OwnerID == "" && ak.OwnerID != "" {
				brec.OwnerID = ak.OwnerID
				changed = true
			}
			if brec.Owner == "" && ak.Owner != "" {
				brec.Owner = ak.Owner
				changed = true
			}
			if ak.OwnerID != "" {
				if u, uerr := h.Svc.Meta.GetUser(ak.OwnerID); uerr == nil {
					if brec.TeamID == "" && u.TeamID != "" {
						brec.TeamID = u.TeamID
						changed = true
					}
					if brec.TenantID == "" && u.TenantID != "" {
						brec.TenantID = u.TenantID
						changed = true
					}
				}
			}
		}
		// x-amz-bucket-object-lock-enabled is only honored at creation. AWS then
		// keeps versioning Enabled; a later PutObjectLockConfiguration is not required.
		if objectLockHeaderOn(r) {
			brec.ObjectLock = true
			brec.Versioning = true
			brec.VersioningSuspended = false
			changed = true
		}
		if changed {
			_ = h.Svc.Meta.UpdateBucket(brec)
		}
	}
	w.Header().Set("Location", "/"+bucket)
	w.WriteHeader(http.StatusOK)
}

func objectLockHeaderOn(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Amz-Bucket-Object-Lock-Enabled")), "true")
}

func (h *Handler) deleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:DeleteBucket") {
		return
	}
	if h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.DeleteBucket(r.Context(), sk); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getBucketLocation answers ?location. An object listing is not a region:
// minio-go reads this before PUT and would put that XML into Authorization.
func (h *Handler) getBucketLocation(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:GetBucketLocation") {
		return
	}
	if _, ok := h.storageBucket(w, r, creds, bucket); !ok {
		return
	}
	region := h.Svc.Region
	// us-east-1 is the empty constraint. Any other region is the element text.
	text := region
	if region == "" || region == "us-east-1" {
		text = ""
	}
	writeXML(w, http.StatusOK, locationConstraint{Xmlns: xmlNS, Region: text})
}

type locationConstraint struct {
	XMLName xml.Name `xml:"LocationConstraint"`
	Xmlns   string   `xml:"xmlns,attr"`
	Region  string   `xml:",chardata"`
}

func (h *Handler) headBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:ListBucket") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if _, err := h.Svc.Meta.GetBucketByKey(sk); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) listObjects(w http.ResponseWriter, r *http.Request, bucket string, q map[string][]string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:ListBucket") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	prefix := first(q["prefix"])
	if eff, ok := h.Svc.EffectiveListPrefixForAccessKey(creds.AccessKey, sk, prefix); !ok {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
		return
	} else {
		prefix = eff
	}
	maxKeys := parseMaxKeys(first(q["max-keys"]))
	v2 := first(q["list-type"]) == "2"
	start := first(q["marker"])
	if v2 {
		if tok := first(q["continuation-token"]); tok != "" {
			start = tok
		} else {
			start = first(q["start-after"])
		}
	}
	objs, truncated, next, err := h.listObjectPage(sk, prefix, start, maxKeys)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	objs = h.Svc.FilterObjectsForAccessKey(creds.AccessKey, sk, objs)
	contents := h.objectInfos(sk, objs)
	if v2 {
		resp := ListObjectsV2Result{
			Xmlns:       xmlNS,
			Name:        bucket,
			Prefix:      prefix,
			KeyCount:    len(contents),
			MaxKeys:     maxKeys,
			IsTruncated: truncated,
			Contents:    contents,
		}
		if tok := first(q["continuation-token"]); tok != "" {
			resp.ContinuationToken = tok
		}
		if truncated {
			resp.NextContinuationToken = next
		}
		writeXML(w, http.StatusOK, resp)
		return
	}
	resp := ListBucketResult{
		Xmlns:       xmlNS,
		Name:        bucket,
		Prefix:      prefix,
		Marker:      first(q["marker"]),
		MaxKeys:     maxKeys,
		IsTruncated: truncated,
		Contents:    contents,
	}
	if truncated {
		resp.NextMarker = next
	}
	writeXML(w, http.StatusOK, resp)
}

func (h *Handler) listObjectPage(bucket, prefix, start string, maxKeys int) ([]metadata.ObjectRecord, bool, string, error) {
	if maxKeys == 0 {
		probe, _, _, err := h.Svc.Meta.ListObjectsPage(bucket, prefix, start, 1)
		if err != nil {
			return nil, false, "", err
		}
		return nil, len(probe) > 0, "", nil
	}
	return h.Svc.Meta.ListObjectsPage(bucket, prefix, start, maxKeys)
}

func (h *Handler) objectInfos(storageKey string, objs []metadata.ObjectRecord) []ObjectInfo {
	out := make([]ObjectInfo, 0, len(objs))
	for _, o := range objs {
		sc := metadata.S3StorageClassDisplay(o.StorageClass)
		if sc == metadata.StorageClassStandard {
			if brec, err := h.Svc.Meta.GetBucketByKey(storageKey); err == nil && brec.StorageClass != "" {
				sc = metadata.S3StorageClassDisplay(brec.StorageClass)
			}
		}
		out = append(out, ObjectInfo{
			Key:          o.Key,
			LastModified: formatTime(o.LastModified),
			ETag:         o.ETag,
			Size:         o.Size,
			StorageClass: sc,
			VersionID:    o.VersionID,
		})
	}
	return out
}

func parseMaxKeys(raw string) int {
	if raw == "" {
		return 1000
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 1000
	}
	if n > 1000 {
		return 1000
	}
	return n
}

func (h *Handler) listObjectVersions(w http.ResponseWriter, r *http.Request, bucket string, q map[string][]string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:ListBucket") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	prefix := first(q["prefix"])
	if eff, ok := h.Svc.EffectiveListPrefixForAccessKey(creds.AccessKey, sk, prefix); !ok {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
		return
	} else {
		prefix = eff
	}
	maxKeys := parseMaxKeys(first(q["max-keys"]))
	objs, err := h.Svc.ListObjectVersions(r.Context(), sk, prefix, 0)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	objs = h.Svc.FilterObjectsForAccessKey(creds.AccessKey, sk, objs)
	// The list is newest-first within a key. That first row is IsLatest,
	// including when it is a delete marker. GetObject hides delete markers,
	// so it cannot answer this.
	type latestMark struct {
		id     string
		marker bool
	}
	latest := map[string]latestMark{}
	for _, o := range objs {
		if _, ok := latest[o.Key]; ok {
			continue
		}
		latest[o.Key] = latestMark{id: o.VersionID, marker: o.IsDeleteMarker}
	}
	page, truncated, nextKey, nextVid := pageVersions(objs, first(q["key-marker"]), first(q["version-id-marker"]), maxKeys)
	resp := ListObjectVersionsResult{
		Xmlns:               xmlNS,
		Name:                bucket,
		Prefix:              prefix,
		KeyMarker:           first(q["key-marker"]),
		VersionIDMarker:     first(q["version-id-marker"]),
		KeyCount:            len(page),
		MaxKeys:             maxKeys,
		IsTruncated:         truncated,
		NextKeyMarker:       nextKey,
		NextVersionIDMarker: nextVid,
	}
	for _, o := range page {
		entry := VersionEntry{
			Key:          o.Key,
			LastModified: o.LastModified.UTC().Format(time.RFC3339Nano),
			ETag:         o.ETag,
			Size:         o.Size,
			StorageClass: metadata.S3StorageClassDisplay(o.StorageClass),
			VersionID:    o.VersionID,
		}
		if mark, ok := latest[o.Key]; ok && mark.id == o.VersionID && mark.marker == o.IsDeleteMarker {
			entry.IsLatest = true
		}
		resp.Items = append(resp.Items, orderedEntry{DeleteMarker: o.IsDeleteMarker, Entry: entry})
	}
	writeXML(w, http.StatusOK, resp)
}

func pageVersions(objs []metadata.ObjectRecord, keyMarker, versionMarker string, maxKeys int) (page []metadata.ObjectRecord, truncated bool, nextKey, nextVid string) {
	start := 0
	if keyMarker != "" {
		start = len(objs)
		for i, o := range objs {
			if o.Key < keyMarker {
				continue
			}
			if o.Key > keyMarker {
				start = i
				break
			}
			if versionMarker == "" {
				continue
			}
			if o.VersionID == versionMarker {
				start = i + 1
				break
			}
		}
	}
	rest := objs[start:]
	if len(rest) > maxKeys {
		truncated = true
		rest = rest[:maxKeys]
	}
	if truncated && len(rest) > 0 {
		last := rest[len(rest)-1]
		nextKey = last.Key
		nextVid = last.VersionID
	}
	return rest, truncated, nextKey, nextVid
}

func (h *Handler) validObjectKey(w http.ResponseWriter, r *http.Request, key string) bool {
	if err := storage.ValidateObjectKey(key); err != nil {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", err.Error())
		return false
	}
	return true
}

func (h *Handler) putObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.validObjectKey(w, r, key) {
		return
	}
	if h.rejectBlockedPresign(w, r, creds, bucket) {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	body, size, chunkErr := auth.UnwrapAWSChunked(r, creds.SecretKey)
	if chunkErr != nil {
		mapErr(w, r, chunkErr)
		return
	}
	ctx, sseErr := ctxWithSSE(WithObjectEvents(r.Context()), r)
	if sseErr != nil {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", "SSEAlgorithm must be AES256 or aws:kms")
		return
	}
	rec, err := h.Svc.PutObject(ctx, sk, key, body, size, contentType, userMetadata(r.Header))
	if err != nil {
		mapErr(w, r, err)
		return
	}
	applySSEHeaders(w, rec)
	w.Header().Set("ETag", rec.ETag)
	if rec.VersionID != "" {
		w.Header().Set("x-amz-version-id", rec.VersionID)
	}
	w.WriteHeader(http.StatusOK)
	observability.IncS3WriteOps(bucket)
}

func (h *Handler) getObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if !h.validObjectKey(w, r, key) {
		return
	}
	creds, authErr := h.Svc.Signer.Authenticate(r)
	if authErr != nil {
		if errors.Is(authErr, auth.ErrMissingAuth) && h.serveAnonymousGet(w, r, bucket, key) {
			observability.IncS3ReadOps(bucket)
			return
		}
		h.writeAuthError(w, r, authErr)
		return
	}
	if h.rejectBlockedPresign(w, r, creds, bucket) {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:GetObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if h.rejectKMSPresign(w, r, creds, sk, key) {
		return
	}
	h.serveObject(w, r, sk, key)
	observability.IncS3ReadOps(bucket)
}

func (h *Handler) serveAnonymousGet(w http.ResponseWriter, r *http.Request, logicalBucket, key string) bool {
	rec, err := h.Svc.ResolvePublicRead(logicalBucket, tenantHint(r), requestSourceIP(r), r.TLS != nil, key)
	if err != nil {
		if errors.Is(err, ErrPublicBlocked) {
			h.notePublicRead(r, logicalBucket, key, "public_blocked")
			writeError(w, r, http.StatusForbidden, "AccessDenied", "public access is blocked by policy")
			return true
		}
		if errors.Is(err, ErrPublicAmbiguous) {
			writeError(w, r, http.StatusNotFound, "NoSuchBucket", "bucket not found")
			return true
		}
		return false
	}
	if h.rejectAnonymousKMS(w, r, rec.EffectiveStorageKey(), logicalBucket, key) {
		return true
	}
	h.notePublicRead(r, logicalBucket, key, "ok")
	h.serveObject(w, r, rec.EffectiveStorageKey(), key)
	return true
}

func (h *Handler) serveAnonymousHead(w http.ResponseWriter, r *http.Request, logicalBucket, key string) bool {
	rec, err := h.Svc.ResolvePublicRead(logicalBucket, tenantHint(r), requestSourceIP(r), r.TLS != nil, key)
	if err != nil {
		if errors.Is(err, ErrPublicBlocked) {
			writeError(w, r, http.StatusForbidden, "AccessDenied", "public access is blocked by policy")
			return true
		}
		if errors.Is(err, ErrPublicAmbiguous) {
			writeError(w, r, http.StatusNotFound, "NoSuchBucket", "bucket not found")
			return true
		}
		return false
	}
	if h.rejectAnonymousKMS(w, r, rec.EffectiveStorageKey(), logicalBucket, key) {
		return true
	}
	h.serveHeadObject(w, r, rec.EffectiveStorageKey(), key)
	return true
}

func (h *Handler) rejectBlockedPresign(w http.ResponseWriter, r *http.Request, creds auth.Credentials, bucket string) bool {
	if r.URL.Query().Get("X-Amz-Algorithm") == "" {
		return false
	}
	if msg := h.Svc.PresignBlockReason(creds.AccessKey, bucket); msg != "" {
		if h.OnPresignBlocked != nil {
			h.OnPresignBlocked(r.Context(), creds.AccessKey, r.RemoteAddr, bucket, msg)
		}
		writeError(w, r, http.StatusForbidden, "AccessDenied", msg)
		return true
	}
	return false
}

func (h *Handler) serveObject(w http.ResponseWriter, r *http.Request, storageBucket, key string) {
	versionID := r.URL.Query().Get("versionId")
	rc, rec, err := h.Svc.GetObject(r.Context(), storageBucket, key, versionID)
	if err != nil {
		if errors.Is(err, ErrKeyDestroyed) {
			h.notePresignDecrypt(r, storageBucket, key, rec, "key_destroyed")
		}
		if errors.Is(err, ErrKMSUnavailable) {
			h.notePresignDecrypt(r, storageBucket, key, rec, "kms_unavailable")
		}
		if strings.Contains(err.Error(), "message authentication failed") {
			w.Header().Set("Connection", "close")
		}
		mapErr(w, r, err)
		return
	}
	defer rc.Close()
	if rec.Encryption != nil && rec.Encryption.Sealed() {
		h.notePresignDecrypt(r, storageBucket, key, rec, "ok")
	}
	applySSEHeaders(w, rec)
	writeObjectMeta(w, rec)
	w.Header().Set("ETag", rec.ETag)
	if rec.VersionID != "" {
		w.Header().Set("x-amz-version-id", rec.VersionID)
	}
	w.Header().Set("Last-Modified", rec.LastModified.UTC().Format(http.TimeFormat))
	if rec.ContentType != "" {
		w.Header().Set("Content-Type", rec.ContentType)
	}
	start, end, ok := ParseRange(r.Header.Get("Range"), rec.Size)
	if r.Header.Get("Range") != "" {
		if !ok {
			writeError(w, r, http.StatusRequestedRangeNotSatisfiable, "InvalidRange", "invalid range")
			return
		}
		if seeker, canSeek := rc.(io.ReadSeeker); canSeek {
			_, _ = seeker.Seek(start, io.SeekStart)
		} else {
			_, _ = io.CopyN(io.Discard, rc, start)
		}
		length := end - start + 1
		w.Header().Set("Content-Range", ContentRange(start, end, rec.Size))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.CopyN(w, rc, length)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(rec.Size, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
}

func (h *Handler) writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case auth.ErrUnsignedHeaders:
		writeError(w, r, http.StatusBadRequest, "AccessDenied", "There were headers present in the request which were not signed")
	case auth.ErrMissingAuth, auth.ErrInvalidAuth, auth.ErrSignatureMismatch:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
	case auth.ErrExpired:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "request expired")
	default:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
	}
}

func (h *Handler) headObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if !h.validObjectKey(w, r, key) {
		return
	}
	creds, authErr := h.Svc.Signer.Authenticate(r)
	if authErr != nil {
		if errors.Is(authErr, auth.ErrMissingAuth) && h.serveAnonymousHead(w, r, bucket, key) {
			observability.IncS3ReadOps(bucket)
			return
		}
		h.writeAuthError(w, r, authErr)
		return
	}
	if h.rejectBlockedPresign(w, r, creds, bucket) {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:GetObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	h.serveHeadObject(w, r, sk, key)
	observability.IncS3ReadOps(bucket)
}

func (h *Handler) serveHeadObject(w http.ResponseWriter, r *http.Request, storageBucket, key string) {
	rec, err := h.Svc.HeadObject(r.Context(), storageBucket, key, r.URL.Query().Get("versionId"))
	if err != nil {
		mapErr(w, r, err)
		return
	}
	applySSEHeaders(w, rec)
	writeObjectMeta(w, rec)
	w.Header().Set("ETag", rec.ETag)
	if rec.VersionID != "" {
		w.Header().Set("x-amz-version-id", rec.VersionID)
	}
	w.Header().Set("Content-Length", strconv.FormatInt(rec.Size, 10))
	w.Header().Set("Last-Modified", rec.LastModified.UTC().Format(http.TimeFormat))
	if rec.ContentType != "" {
		w.Header().Set("Content-Type", rec.ContentType)
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	if !h.validObjectKey(w, r, key) {
		return
	}
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if r.URL.Query().Get("X-Amz-Algorithm") != "" {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "presigned DELETE is not allowed")
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:DeleteObject") {
		return
	}
	versionID := r.URL.Query().Get("versionId")
	if versionID != "" && !h.authorize(w, r, creds, bucket, key, "s3:DeleteObjectVersion") {
		return
	}
	bypass, ok := h.governanceBypass(w, r, creds, bucket, key)
	if !ok {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	ctx := WithUserMetadata(WithObjectEvents(r.Context()), userMetadata(r.Header))
	if _, err := h.Svc.DeleteObjectResult(ctx, sk, key, versionID, DeleteOptions{BypassGovernance: bypass}); err != nil {
		if err == metadata.ErrRetentionLocked || err == metadata.ErrLegalHold {
			h.emitActivity(r, creds.AccessKey, metadata.ActionObjectDeleteBlocked, "object", bucket+"/"+key)
		}
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) notePresignDecrypt(r *http.Request, storageBucket, key string, rec metadata.ObjectRecord, result string) {
	if h.OnPresignDecrypt == nil || r.URL.Query().Get("X-Amz-Algorithm") == "" {
		return
	}
	logical := storageBucket
	tenant := ""
	if b, err := h.Svc.Meta.GetBucketByKey(storageBucket); err == nil {
		if b.Name != "" {
			logical = b.Name
		}
		tenant = b.EffectiveTenantID()
	}
	keyID, ver := "", 0
	if rec.Encryption != nil {
		keyID = rec.Encryption.KeyID
		ver = rec.Encryption.KeyVersion
	}
	objectKey := rec.Key
	if objectKey == "" {
		objectKey = key
	}
	user := ""
	if creds, err := h.Svc.Signer.Authenticate(r); err == nil {
		user = creds.AccessKey
	}
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	h.OnPresignDecrypt(r.Context(), user, ip, logical, objectKey, keyID, tenant, result, ver)
}

func (h *Handler) notePublicRead(r *http.Request, logicalBucket, key, result string) {
	if h.OnPublicRead == nil {
		return
	}
	keyID := ""
	ver := 0
	if b, err := h.Svc.Meta.GetBucket(logicalBucket); err == nil {
		if rec, err := h.Svc.Meta.GetObject(b.EffectiveStorageKey(), key); err == nil && rec.Encryption != nil {
			keyID = rec.Encryption.KeyID
			ver = rec.Encryption.KeyVersion
		}
	}
	h.OnPublicRead(r.Context(), requestSourceIP(r), logicalBucket, key, result, keyID, ver)
}

func (h *Handler) emitActivity(r *http.Request, accessKey, action, resourceType, resourceName string) {
	if h.OnActivity == nil {
		return
	}
	user := accessKey
	if accessKey != "" && accessKey == h.Svc.OwnerKey {
		user = "bootstrap"
	}
	if ak, err := h.Svc.Meta.GetAccessKey(accessKey); err == nil {
		if ak.Owner != "" {
			user = ak.Owner
		} else if ak.OwnerID != "" {
			if u, uerr := h.Svc.Meta.GetUser(ak.OwnerID); uerr == nil && u.Username != "" {
				user = u.Username
			}
		}
	}
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.TrimSpace(strings.Split(fwd, ",")[0])
	} else if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip = strings.TrimSpace(xri)
	}
	h.OnActivity(user, ip, action, resourceType, resourceName)
}

func (h *Handler) copyObject(w http.ResponseWriter, r *http.Request, dstBucket, dstKey string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, dstBucket, dstKey, "s3:PutObject") {
		return
	}
	src := r.Header.Get("x-amz-copy-source")
	srcBucket, srcKey, ok := copySourceBucketKey(src)
	if !ok {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", "invalid copy source")
		return
	}
	dstSK, ok := h.storageBucket(w, r, creds, dstBucket)
	if !ok {
		return
	}
	srcSK, ok := h.storageBucket(w, r, creds, srcBucket)
	if !ok {
		return
	}
	ctx, sseErr := ctxWithSSE(WithObjectEvents(r.Context()), r)
	if sseErr != nil {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", "SSEAlgorithm must be AES256 or aws:kms")
		return
	}
	rec, err := h.Svc.CopyObject(ctx, srcSK, srcKey, dstSK, dstKey)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	applySSEHeaders(w, rec)
	w.Header().Set("ETag", rec.ETag)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) listMultipartUploads(w http.ResponseWriter, r *http.Request, bucket string, q map[string][]string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:ListBucketMultipartUploads") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rows, err := h.Svc.ListMultipartUploads(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	prefix := first(q["prefix"])
	keyMarker := first(q["key-marker"])
	idMarker := first(q["upload-id-marker"])
	maxUploads := parseMaxKeys(first(q["max-uploads"]))
	var matched []metadata.MultipartRecord
	for _, rec := range rows {
		if prefix != "" && !strings.HasPrefix(rec.Key, prefix) {
			continue
		}
		if keyMarker != "" {
			if rec.Key < keyMarker {
				continue
			}
			if rec.Key == keyMarker && (idMarker == "" || rec.UploadID <= idMarker) {
				continue
			}
		}
		matched = append(matched, rec)
	}
	truncated := false
	if maxUploads >= 0 && len(matched) > maxUploads {
		truncated = true
		matched = matched[:maxUploads]
	}
	resp := ListMultipartUploadsResult{
		Xmlns:          xmlNS,
		Bucket:         bucket,
		Prefix:         prefix,
		KeyMarker:      keyMarker,
		UploadIDMarker: idMarker,
		MaxUploads:     maxUploads,
		IsTruncated:    truncated,
	}
	for _, rec := range matched {
		resp.Upload = append(resp.Upload, MultipartUploadEntry{
			Key:          rec.Key,
			UploadID:     rec.UploadID,
			StorageClass: "STANDARD",
			Initiated:    formatTime(rec.Initiated),
		})
	}
	if truncated && len(matched) > 0 {
		last := matched[len(matched)-1]
		resp.NextKeyMarker = last.Key
		resp.NextUploadIDMarker = last.UploadID
	}
	writeXML(w, http.StatusOK, resp)
}

func (h *Handler) createMultipart(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	ctx, sseErr := ctxWithSSE(r.Context(), r)
	if sseErr != nil {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", "SSEAlgorithm must be AES256 or aws:kms")
		return
	}
	ctx = WithUserMetadata(ctx, userMetadata(r.Header))
	if ct := r.Header.Get("Content-Type"); ct != "" {
		ctx = WithContentType(ctx, ct)
	}
	uploadID, err := h.Svc.CreateMultipartUpload(ctx, sk, key)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	resp := InitiateMultipartUploadResult{
		Xmlns:    xmlNS,
		Bucket:   bucket,
		Key:      key,
		UploadID: uploadID,
	}
	writeXML(w, http.StatusOK, resp)
}

func (h *Handler) uploadPart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string, q map[string][]string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	partNum, err := strconv.Atoi(first(q["partNumber"]))
	if err != nil || partNum <= 0 {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", "invalid part number")
		return
	}
	body, size, chunkErr := auth.UnwrapAWSChunked(r, creds.SecretKey)
	if chunkErr != nil {
		mapErr(w, r, chunkErr)
		return
	}
	etag, err := h.Svc.UploadPart(r.Context(), sk, key, uploadID, partNum, body, size)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	w.Header().Set("ETag", etag)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) completeMultipart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObject") {
		return
	}
	var req CompleteMultipartUpload
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	var parts []storage.PartInfo
	for _, p := range req.Part {
		parts = append(parts, storage.PartInfo{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rec, err := h.Svc.CompleteMultipartUpload(WithObjectEvents(r.Context()), sk, key, uploadID, parts)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	if rec.VersionID != "" {
		w.Header().Set("x-amz-version-id", rec.VersionID)
	}
	resp := CompleteMultipartUploadResult{
		Xmlns:    xmlNS,
		Location: location(bucket, key),
		Bucket:   bucket,
		Key:      key,
		ETag:     rec.ETag,
	}
	writeXML(w, http.StatusOK, resp)
}

func (h *Handler) abortMultipart(w http.ResponseWriter, r *http.Request, bucket, key, uploadID string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.AbortMultipartUpload(r.Context(), sk, key, uploadID); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getObjectTagging(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:GetObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rec, err := h.Svc.GetObjectTags(r.Context(), sk, key, r.URL.Query().Get("versionId"))
	if err != nil {
		mapErr(w, r, err)
		return
	}
	writeXML(w, http.StatusOK, Tagging{TagSet: TagSet{Tags: mapToTagEntries(rec)}})
}

func (h *Handler) putObjectTagging(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:PutObject") {
		return
	}
	var req Tagging
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.SetObjectTags(r.Context(), sk, key, r.URL.Query().Get("versionId"), tagsToMap(req.TagSet.Tags)); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func writeObjectMeta(w http.ResponseWriter, rec metadata.ObjectRecord) {
	if rec.IsDeleteMarker {
		w.Header().Set("x-amz-delete-marker", "true")
	}
	for k, v := range rec.Metadata {
		if strings.HasPrefix(strings.ToLower(k), "x-amz-meta-") {
			w.Header().Set(k, v)
		}
	}
}

func userMetadata(h http.Header) map[string]string {
	out := map[string]string{}
	for k, vals := range h {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-amz-meta-") && len(vals) > 0 {
			out[k] = vals[0]
		}
	}
	return out
}

func first(v []string) string {
	if len(v) == 0 {
		return ""
	}
	return v[0]
}

func writeXML(w http.ResponseWriter, status int, v any) {
	data, err := marshalXML(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	resp := ErrorResponse{
		Code:      code,
		Message:   message,
		Resource:  r.URL.Path,
		RequestID: time.Now().UTC().Format("20060102T150405Z"),
	}
	data, _ := marshalXML(resp)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func mapErr(w http.ResponseWriter, r *http.Request, err error) {
	switch err {
	case metadata.ErrNotFound, storage.ErrNotFound:
		if strings.Count(strings.Trim(r.URL.Path, "/"), "/") == 0 {
			writeError(w, r, http.StatusNotFound, "NoSuchBucket", "bucket does not exist")
		} else {
			writeError(w, r, http.StatusNotFound, "NoSuchKey", "key does not exist")
		}
	case metadata.ErrBucketExists, storage.ErrBucketExists:
		writeError(w, r, http.StatusConflict, "BucketAlreadyOwnedByYou", "bucket already exists")
	case metadata.ErrBucketNotEmpty, storage.ErrBucketNotEmpty:
		writeError(w, r, http.StatusConflict, "BucketNotEmpty", "bucket not empty")
	case metadata.ErrQuotaExceeded:
		writeError(w, r, http.StatusForbidden, "QuotaExceeded", "quota exceeded")
	case ErrSSEKeyMissing:
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", ErrSSEKeyMissing.Error())
	case ErrPublicKMS:
		writeError(w, r, http.StatusConflict, "InvalidRequest", MsgPublicKMS)
	case ErrClusterUpgrade:
		writeError(w, r, http.StatusConflict, "InvalidRequest", MsgClusterUpgrade)
	case ErrKeyNotAllowed:
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", ErrKeyNotAllowed.Error())
	case ErrKeyDestroyed:
		writeError(w, r, http.StatusForbidden, "AccessDenied", MsgKeyDestroyed)
	case ErrKMSUnavailable:
		writeError(w, r, http.StatusServiceUnavailable, "ServiceUnavailable", "KMS unavailable")
	case ErrObjectUnavailable:
		w.Header().Set("Retry-After", "1")
		writeError(w, r, http.StatusServiceUnavailable, "ServiceUnavailable", ErrObjectUnavailable.Error())
	case errObjectLockVersioning:
		writeError(w, r, http.StatusConflict, "InvalidBucketState", errObjectLockVersioning.Error())
	case metadata.ErrLegalHold:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "object is under legal hold")
	case metadata.ErrRetentionLocked:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "object retention period has not expired")
	case auth.ErrChunkSignature:
		writeError(w, r, http.StatusForbidden, "SignatureDoesNotMatch", "The request signature we calculated does not match the signature you provided")
	case auth.ErrBadDigest:
		writeError(w, r, http.StatusBadRequest, "BadDigest", "The checksum you specified did not match")
	case auth.ErrMalformedChunk, auth.ErrDecodedLength:
		writeError(w, r, http.StatusBadRequest, "IncompleteBody", err.Error())
	default:
		// A leader kill leaves in-flight writes on a session Postgres has already
		// made read-only (SQLSTATE 25006) or drops the connection (57P01, 08006,
		// reset). Those were HTTP 500 InternalError. Clients retry 503
		// ServiceUnavailable with this message; the body is already consumed, so
		// this process does not replay the PUT. A follower that cannot dial the
		// previous holder answers the same 503 from leaderGuard.
		if retryableClusterWrite(err) {
			w.Header().Set("Retry-After", "1")
			writeError(w, r, http.StatusServiceUnavailable, "ServiceUnavailable", "not cluster leader")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "InternalError", err.Error())
	}
}

func retryableClusterWrite(err error) bool {
	if err == nil {
		return false
	}
	if events.StoreFenced(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, n := range []string{
		"sqlstate 57p01",
		"sqlstate 57p02",
		"sqlstate 57p03",
		"sqlstate 08000",
		"sqlstate 08001",
		"sqlstate 08003",
		"sqlstate 08006",
		"sqlstate 40001",
		"sqlstate 40p01",
		"connection refused",
		"connection reset",
		"broken pipe",
		"server closed the connection",
		"conn closed",
		"terminating connection",
	} {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}
