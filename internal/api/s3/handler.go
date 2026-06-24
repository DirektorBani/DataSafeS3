package s3

import (
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/storage"
)

type Handler struct {
	Svc *Service
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
			if q.Has("notification") {
				h.deleteBucketNotification(w, r, bucket)
				return
			}
			h.deleteBucket(w, r, bucket)
		case http.MethodGet:
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
			if q.Has("versions") {
				h.listObjectVersions(w, r, bucket, q)
				return
			}
			h.listObjects(w, r, bucket, q)
		case http.MethodHead:
			h.headBucket(w, r, bucket)
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
	if !h.Svc.Authorize(bucket, key, action, creds.AccessKey) {
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
	var cfg LifecycleConfiguration
	if err := xml.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rules := lifecycleRulesFromXML(cfg)
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
		if ak, aerr := h.Svc.Meta.GetAccessKey(creds.AccessKey); aerr == nil {
			changed := false
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
			if changed {
				_ = h.Svc.Meta.UpdateBucket(brec)
			}
		}
	}
	w.Header().Set("Location", "/"+bucket)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteBucket(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:DeleteBucket") {
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
	maxKeys := 1000
	if mk := first(q["max-keys"]); mk != "" {
		if n, err := strconv.Atoi(mk); err == nil && n > 0 {
			maxKeys = n
		}
	}
	objs, err := h.Svc.ListObjects(r.Context(), sk, prefix, maxKeys)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	objs = h.Svc.FilterObjectsForAccessKey(creds.AccessKey, sk, objs)
	resp := ListObjectsV2Result{
		Xmlns:       xmlNS,
		Name:        bucket,
		Prefix:      prefix,
		KeyCount:    len(objs),
		MaxKeys:     maxKeys,
		IsTruncated: false,
	}
	for _, o := range objs {
		sc := metadata.S3StorageClassDisplay(o.StorageClass)
		if sc == metadata.StorageClassStandard {
			if brec, err := h.Svc.Meta.GetBucketByKey(sk); err == nil && brec.StorageClass != "" {
				sc = metadata.S3StorageClassDisplay(brec.StorageClass)
			}
		}
		resp.Contents = append(resp.Contents, ObjectInfo{
			Key:          o.Key,
			LastModified: formatTime(o.LastModified),
			ETag:         o.ETag,
			Size:         o.Size,
			StorageClass: sc,
			VersionID:    o.VersionID,
		})
	}
	writeXML(w, http.StatusOK, resp)
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
	maxKeys := 1000
	if mk := first(q["max-keys"]); mk != "" {
		if n, err := strconv.Atoi(mk); err == nil && n > 0 {
			maxKeys = n
		}
	}
	objs, err := h.Svc.ListObjectVersions(r.Context(), sk, prefix, maxKeys)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	objs = h.Svc.FilterObjectsForAccessKey(creds.AccessKey, sk, objs)
	resp := ListObjectVersionsResult{
		Xmlns:       xmlNS,
		Name:        bucket,
		Prefix:      prefix,
		KeyCount:    len(objs),
		MaxKeys:     maxKeys,
		IsTruncated: false,
	}
	for _, o := range objs {
		sc := metadata.S3StorageClassDisplay(o.StorageClass)
		resp.Versions = append(resp.Versions, ObjectInfo{
			Key:          o.Key,
			LastModified: formatTime(o.LastModified),
			ETag:         o.ETag,
			Size:         o.Size,
			StorageClass: sc,
			VersionID:    o.VersionID,
		})
	}
	writeXML(w, http.StatusOK, resp)
}

func (h *Handler) putObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
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
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	size := r.ContentLength
	rec, err := h.Svc.PutObject(r.Context(), sk, key, r.Body, size, contentType, userMetadata(r.Header))
	if err != nil {
		mapErr(w, r, err)
		return
	}
	w.Header().Set("ETag", rec.ETag)
	if rec.VersionID != "" {
		w.Header().Set("x-amz-version-id", rec.VersionID)
	}
	w.WriteHeader(http.StatusOK)
	observability.IncS3WriteOps(bucket)
}

func (h *Handler) getObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, authErr := h.Svc.Signer.Authenticate(r)
	if authErr != nil {
		if errors.Is(authErr, auth.ErrMissingAuth) && h.Svc.IsPublicReadBucket(bucket) {
			h.serveObjectPublic(w, r, bucket, key)
			observability.IncS3ReadOps(bucket)
			return
		}
		h.writeAuthError(w, r, authErr)
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:GetObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	h.serveObject(w, r, sk, key)
	observability.IncS3ReadOps(bucket)
}

func (h *Handler) serveObjectPublic(w http.ResponseWriter, r *http.Request, logicalBucket, key string) {
	sk, err := h.Svc.PublicReadStorageKey(logicalBucket)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	h.serveObject(w, r, sk, key)
}

func (h *Handler) serveObject(w http.ResponseWriter, r *http.Request, storageBucket, key string) {
	versionID := r.URL.Query().Get("versionId")
	rc, rec, err := h.Svc.GetObject(r.Context(), storageBucket, key, versionID)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	defer rc.Close()
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
	case auth.ErrMissingAuth, auth.ErrInvalidAuth, auth.ErrSignatureMismatch:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
	case auth.ErrExpired:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "request expired")
	default:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "access denied")
	}
}

func (h *Handler) headObject(w http.ResponseWriter, r *http.Request, bucket, key string) {
	creds, authErr := h.Svc.Signer.Authenticate(r)
	if authErr != nil {
		if errors.Is(authErr, auth.ErrMissingAuth) && h.Svc.IsPublicReadBucket(bucket) {
			sk, err := h.Svc.PublicReadStorageKey(bucket)
			if err != nil {
				mapErr(w, r, err)
				return
			}
			h.serveHeadObject(w, r, sk, key)
			observability.IncS3ReadOps(bucket)
			return
		}
		h.writeAuthError(w, r, authErr)
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
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, key, "s3:DeleteObject") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.DeleteObject(r.Context(), sk, key, r.URL.Query().Get("versionId")); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	rec, err := h.Svc.CopyObject(r.Context(), srcSK, srcKey, dstSK, dstKey)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	w.Header().Set("ETag", rec.ETag)
	w.WriteHeader(http.StatusOK)
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
	uploadID, err := h.Svc.CreateMultipartUpload(r.Context(), sk, key)
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
	etag, err := h.Svc.UploadPart(r.Context(), sk, key, uploadID, partNum, r.Body, r.ContentLength)
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
	rec, err := h.Svc.CompleteMultipartUpload(r.Context(), sk, key, uploadID, parts)
	if err != nil {
		mapErr(w, r, err)
		return
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
	case metadata.ErrLegalHold:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "object is under legal hold")
	case metadata.ErrRetentionLocked:
		writeError(w, r, http.StatusForbidden, "AccessDenied", "object retention period has not expired")
	default:
		writeError(w, r, http.StatusInternalServerError, "InternalError", err.Error())
	}
}
