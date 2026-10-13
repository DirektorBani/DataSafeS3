package s3

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"

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
	rec.DefaultEncryption = algo
	rec.KMSMasterKeyID = cfg.Rule.Apply.KMSMasterKeyID
	if err := h.Svc.Meta.UpdateBucket(rec); err != nil {
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
	rec.DefaultEncryption = ""
	rec.KMSMasterKeyID = ""
	if err := h.Svc.Meta.UpdateBucket(rec); err != nil {
		mapErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
