package s3

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/policy"
)

type encryptionConfiguration struct {
	XMLName xml.Name        `xml:"ServerSideEncryptionConfiguration"`
	Xmlns   string          `xml:"xmlns,attr,omitempty"`
	Rule    *encryptionRule `xml:"Rule,omitempty"`
}

type encryptionRule struct {
	Apply encryptionDefault `xml:"ApplyServerSideEncryptionByDefault"`
}

type encryptionDefault struct {
	SSEAlgorithm   string `xml:"SSEAlgorithm"`
	KMSMasterKeyID string `xml:"KMSMasterKeyID,omitempty"`
}

func (h *Handler) putBucketEncryption(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketEncryption") {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid xml")
		return
	}
	var cfg encryptionConfiguration
	if err := xml.Unmarshal(body, &cfg); err != nil || cfg.Rule == nil {
		writeError(w, r, http.StatusBadRequest, "MalformedXML", "invalid encryption configuration")
		return
	}
	algo := strings.TrimSpace(cfg.Rule.Apply.SSEAlgorithm)
	if algo != "AES256" && algo != "aws:kms" {
		writeError(w, r, http.StatusBadRequest, "InvalidArgument", "SSEAlgorithm must be AES256 or aws:kms")
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rec, err := h.Svc.Meta.GetBucketByKey(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	if _, err := h.Svc.SetBucketEncryption(rec, algo, cfg.Rule.Apply.KMSMasterKeyID); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) getBucketEncryption(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:GetBucketEncryption") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rec, err := h.Svc.Meta.GetBucketByKey(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	if rec.DefaultEncryption == "" {
		writeError(w, r, http.StatusNotFound, "ServerSideEncryptionConfigurationNotFoundError", "encryption configuration not found")
		return
	}
	writeXML(w, http.StatusOK, encryptionConfiguration{
		Xmlns: xmlNS,
		Rule: &encryptionRule{Apply: encryptionDefault{
			SSEAlgorithm: rec.DefaultEncryption, KMSMasterKeyID: rec.KMSMasterKeyID,
		}},
	})
}

func (h *Handler) deleteBucketEncryption(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketEncryption") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	rec, err := h.Svc.Meta.GetBucketByKey(sk)
	if err != nil {
		mapErr(w, r, err)
		return
	}
	if _, err := h.Svc.ClearBucketEncryption(rec); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func ctxWithSSE(ctx context.Context, r *http.Request) (context.Context, error) {
	algo := strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption"))
	keyID := strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption-aws-kms-key-id"))
	if algo == "" {
		return ctx, nil
	}
	if algo != "AES256" && algo != "aws:kms" {
		return ctx, errors.New("invalid sse")
	}
	return WithSSERequest(ctx, algo, keyID), nil
}

func applySSEHeaders(w http.ResponseWriter, rec metadata.ObjectRecord) {
	if rec.Encryption == nil || !rec.Encryption.Sealed() {
		return
	}
	if rec.Encryption.SSEAlgorithm != "" {
		w.Header().Set("x-amz-server-side-encryption", rec.Encryption.SSEAlgorithm)
	}
	if rec.Encryption.KMS() && rec.Encryption.KeyID != "" {
		w.Header().Set("x-amz-server-side-encryption-aws-kms-key-id", rec.Encryption.KeyID)
	}
}

func (h *Handler) rejectAnonymousKMS(w http.ResponseWriter, r *http.Request, bucket, key string) bool {
	rec, err := h.Svc.Meta.GetObject(bucket, key)
	if err != nil || rec.Encryption == nil || !rec.Encryption.KMS() {
		return false
	}
	writeError(w, r, http.StatusForbidden, "AccessDenied", MsgKMSPublic)
	return true
}

func (h *Handler) rejectKMSPresign(w http.ResponseWriter, r *http.Request, creds auth.Credentials, bucket, key string) bool {
	if r.URL.Query().Get("X-Amz-Algorithm") == "" {
		return false
	}
	rec, err := h.Svc.Meta.GetObject(bucket, key)
	if err != nil || rec.Encryption == nil || !rec.Encryption.Sealed() {
		return false
	}
	if creds.AccessKey == "" || creds.AccessKey == h.Svc.OwnerKey {
		return false
	}
	ak, err := h.Svc.Meta.GetAccessKey(creds.AccessKey)
	if err != nil {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "signer is not allowed to read this encrypted object")
		return true
	}
	var user metadata.UserRecord
	var uerr error
	switch {
	case ak.OwnerID != "":
		user, uerr = h.Svc.Meta.GetUser(ak.OwnerID)
	case ak.Owner != "":
		user, uerr = h.Svc.Meta.GetUserByUsername(ak.Owner)
	default:
		return false
	}
	if uerr != nil || user.Status != metadata.StatusActive {
		writeError(w, r, http.StatusForbidden, "AccessDenied", "signer is not allowed to read this encrypted object")
		return true
	}
	return false
}

func (h *Handler) putBucketPolicy(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:PutBucketPolicy") {
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedPolicy", "invalid policy")
		return
	}
	if err := policy.Validate(string(body)); err != nil {
		writeError(w, r, http.StatusBadRequest, "MalformedPolicy", err.Error())
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.Meta.SetBucketPolicy(sk, string(body)); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) deleteBucketPolicy(w http.ResponseWriter, r *http.Request, bucket string) {
	creds, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	if h.denyAdminS3WithoutMFA(w, r, creds.AccessKey) {
		return
	}
	if !h.authorize(w, r, creds, bucket, "", "s3:DeleteBucketPolicy") {
		return
	}
	sk, ok := h.storageBucket(w, r, creds, bucket)
	if !ok {
		return
	}
	if err := h.Svc.Meta.SetBucketPolicy(sk, ""); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
