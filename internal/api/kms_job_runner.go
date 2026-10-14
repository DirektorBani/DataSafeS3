package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"io"
	"sort"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
)

const kmsJobBatch = 32

func (s *Server) runKMSJobs(ctx context.Context) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if !s.kmsJobLeader(ctx) {
				continue
			}
			s.stepKMSJobs(ctx)
		}
	}
}

func (s *Server) kmsJobLeader(ctx context.Context) bool {
	if s.haLeader == nil || !s.haLeader.Enabled() {
		return true
	}
	return s.haLeader.IsLeader(ctx)
}

func (s *Server) kmsJobHolder() string {
	if s.cfg.NodeID != "" {
		return s.cfg.NodeID
	}
	return "local"
}

func (s *Server) stepKMSJobs(ctx context.Context) {
	if !s.kmsJobLeader(ctx) {
		return
	}
	jobs, err := s.meta.ListKMSJobs()
	if err != nil {
		return
	}
	for _, job := range jobs {
		if job.Status != kms.JobPending && job.Status != kms.JobRunning {
			continue
		}
		if !s.kmsJobLeader(ctx) {
			return
		}
		if err := s.stepOneKMSJob(ctx, job); err != nil {
			if events.StoreFenced(err) {
				return
			}
		}
	}
}

func (s *Server) stepOneKMSJob(ctx context.Context, job metadata.KMSJob) error {
	items, err := s.kmsWorkItems(job.BucketKey)
	if err != nil {
		return err
	}
	sort.Slice(items, func(i, j int) bool {
		return kms.Cursor(items[i].Key, items[i].VersionID) < kms.Cursor(items[j].Key, items[j].VersionID)
	})
	if job.Total == 0 && job.Cursor == "" {
		for _, it := range items {
			if !it.DeleteMarker {
				job.Total++
			}
		}
	}
	if job.StartedAt.IsZero() {
		job.StartedAt = time.Now().UTC()
	}
	job.Status = kms.JobRunning
	job.HolderID = s.kmsJobHolder()
	var batch []kms.WorkItem
	for _, it := range items {
		if !kms.AfterCursor(job.Cursor, it.Key, it.VersionID) {
			continue
		}
		batch = append(batch, it)
		if len(batch) >= kmsJobBatch {
			break
		}
	}
	if len(batch) == 0 {
		now := time.Now().UTC()
		job.Status = kms.JobCompleted
		job.FinishedAt = &now
		job.UpdatedAt = now
		return s.meta.PutKMSJob(job)
	}
	for _, it := range batch {
		if !s.kmsJobLeader(ctx) {
			job.UpdatedAt = time.Now().UTC()
			return s.meta.PutKMSJob(job)
		}
		result := s.applyKMSWork(ctx, &job, it)
		observability.ObserveKMSJob(job.Type, result)
		if result == "error" {
			job.Errors++
		}
		// Delete markers are not part of Total. Counting them makes the bar pass 100%.
		if !it.DeleteMarker {
			job.Done++
		}
		job.Cursor = kms.Cursor(it.Key, it.VersionID)
		job.UpdatedAt = time.Now().UTC()
		if err := s.meta.PutKMSJob(job); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) kmsWorkItems(bucketKey string) ([]kms.WorkItem, error) {
	vers, err := s.meta.ListObjectVersions(bucketKey, "", 0)
	if err != nil {
		return nil, err
	}
	var out []kms.WorkItem
	for _, rec := range vers {
		item := kms.WorkItem{
			Key: rec.Key, VersionID: rec.VersionID, DeleteMarker: rec.IsDeleteMarker,
		}
		if rec.Encryption != nil {
			item.Sealed = rec.Encryption.Sealed()
			item.KeyID = rec.Encryption.KeyID
			item.KeyVersion = rec.Encryption.KeyVersion
			item.Wrapped = rec.Encryption.WrappedDEK != ""
			item.FormatVersion = rec.Encryption.FormatVersion
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Server) applyKMSWork(ctx context.Context, job *metadata.KMSJob, it kms.WorkItem) string {
	action := kms.Decide(it, job.Type, job.KeyID, job.TargetVersion)
	if action == kms.ActSkip {
		return "skip"
	}
	rec, err := s.meta.GetObjectVersion(job.BucketKey, it.Key, it.VersionID)
	if err != nil {
		s.noteKMSJobError(job, it.Key+": "+err.Error())
		return "error"
	}
	switch action {
	case kms.ActShred:
		if rec.Encryption == nil {
			return "skip"
		}
		rec.Encryption.WrappedDEK = ""
	case kms.ActRewrap:
		if err := s.rewrapObject(&rec, job.TargetVersion); err != nil {
			s.noteKMSJobError(job, it.Key+": "+err.Error())
			return "error"
		}
	case kms.ActEncrypt:
		if err := s.encryptObjectInPlace(ctx, &rec, *job); err != nil {
			s.noteKMSJobError(job, it.Key+": "+err.Error())
			return "error"
		}
	default:
		return "skip"
	}
	if err := saveObjectRecord(s.meta, rec); err != nil {
		s.noteKMSJobError(job, it.Key+": "+err.Error())
		return "error"
	}
	return action
}

func (s *Server) noteKMSJobError(job *metadata.KMSJob, msg string) {
	if len(job.Samples) < 20 {
		job.Samples = append(job.Samples, msg)
	}
}

func saveObjectRecord(meta metadata.MetadataStore, rec metadata.ObjectRecord) error {
	if rec.VersionID != "" {
		return meta.PutObjectVersioned(rec)
	}
	return meta.PutObject(rec)
}

func (s *Server) rewrapObject(rec *metadata.ObjectRecord, target int) error {
	enc := rec.Encryption
	if enc == nil || enc.WrappedDEK == "" || enc.KeyVersion == target {
		return nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc.WrappedDEK)
	if err != nil {
		return err
	}
	var wrapped []byte
	ver := target
	if enc.Provider == kms.ProviderVault {
		v := s.svc.VaultKMS()
		if v == nil {
			return errKMSJob("vault is not configured")
		}
		wrapped, ver, err = v.Rewrap(context.Background(), enc.KeyID, enc.KeyVersion, raw)
		if err != nil {
			return err
		}
	} else {
		if s.svc.Local == nil {
			return errKMSJob("server key is not configured")
		}
		dek, uerr := s.svc.Local.Unwrap(context.Background(), enc.KeyID, enc.KeyVersion, raw)
		if uerr != nil {
			return uerr
		}
		wrapped, ver, err = s.svc.Local.WrapVersion(dek, target)
		for i := range dek {
			dek[i] = 0
		}
		if err != nil {
			return err
		}
	}
	enc.WrappedDEK = base64.StdEncoding.EncodeToString(wrapped)
	enc.KeyVersion = ver
	return nil
}

func (s *Server) encryptObjectInPlace(ctx context.Context, rec *metadata.ObjectRecord, job metadata.KMSJob) error {
	var rc io.ReadCloser
	var err error
	if rec.VersionID != "" {
		rc, _, err = s.backend.GetObjectVersion(ctx, job.BucketKey, rec.Key, rec.VersionID)
	} else {
		rc, _, err = s.backend.GetObject(ctx, job.BucketKey, rec.Key)
	}
	if err != nil {
		return err
	}
	plain, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return err
	}
	dek, err := kms.NewDEK()
	if err != nil {
		return err
	}
	defer func() {
		for i := range dek {
			dek[i] = 0
		}
	}()
	key, err := s.meta.GetKMSKey(job.KeyID)
	if err != nil {
		return err
	}
	var wrapped []byte
	ver := job.TargetVersion
	if key.Provider == kms.ProviderVault {
		v := s.svc.VaultKMS()
		if v == nil {
			return errKMSJob("vault is not configured")
		}
		wrapped, ver, err = v.Wrap(ctx, job.KeyID, dek)
	} else {
		if s.svc.Local == nil {
			return errKMSJob("server key is not configured")
		}
		wrapped, ver, err = s.svc.Local.WrapVersion(dek, job.TargetVersion)
	}
	if err != nil {
		return err
	}
	body, err := kms.EncryptReader(dek, bytes.NewReader(plain), kms.DefaultChunk)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, body); err != nil {
		return err
	}
	var etag string
	if rec.VersionID != "" {
		etag, err = s.backend.PutObjectVersion(ctx, job.BucketKey, rec.Key, rec.VersionID, bytes.NewReader(buf.Bytes()), int64(buf.Len()), rec.ContentType)
	} else {
		etag, err = s.backend.PutObject(ctx, job.BucketKey, rec.Key, bytes.NewReader(buf.Bytes()), int64(buf.Len()), rec.ContentType)
	}
	if err != nil {
		return err
	}
	algo := "AES256"
	if key.Provider == kms.ProviderVault {
		algo = "aws:kms"
	}
	rec.ETag = etag
	rec.Size = int64(len(plain))
	rec.Encryption = &metadata.ObjectEncryption{
		FormatVersion: 1,
		Provider:      key.Provider,
		KeyID:         job.KeyID,
		KeyVersion:    ver,
		WrappedDEK:    base64.StdEncoding.EncodeToString(wrapped),
		Alg:           "AES-256-GCM",
		SSEAlgorithm:  algo,
		PlainSize:     int64(len(plain)),
	}
	return nil
}

type kmsJobError string

func (e kmsJobError) Error() string { return string(e) }

func errKMSJob(msg string) error { return kmsJobError(msg) }

func newKMSJobID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
