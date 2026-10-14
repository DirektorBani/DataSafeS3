package s3

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/storage"
)

const (
	// MsgKMSPublic is the anonymous-read refusal for SSE-KMS objects.
	MsgKMSPublic = "объект зашифрован ключом KMS, публичный доступ невозможен"
	// MsgKeyDestroyed is the permanent refusal after crypto-shred.
	MsgKeyDestroyed = "Ключ шифрования уничтожен, файл недоступен навсегда"
	// MsgPublicKMS is the refusal to combine public-read with a KMS default.
	MsgPublicKMS = "Публичный доступ к файлам, зашифрованным ключом из Vault, невозможен. Сначала сделайте бакет приватным"
	// MsgClusterUpgrade is returned while a node older than 1.6.0 is heartbeating.
	MsgClusterUpgrade = "cluster upgrade in progress"
)

var (
	ErrSSEKeyMissing  = errors.New("STORAGE_SSE_MASTER_KEY is not configured")
	ErrPublicKMS      = errors.New("public access with aws:kms is not allowed")
	ErrClusterUpgrade = errors.New("cluster upgrade in progress")
	ErrKeyNotAllowed  = errors.New("kms key id is not in the allow list")
	ErrKeyDestroyed   = errors.New("key destroyed")
	ErrKMSUnavailable = errors.New("KMS unavailable")
)

type sseSpec struct {
	Algorithm string
	KeyID     string
	Forced    *metadata.ObjectEncryption
}

type sseCtxKey struct{}

// WithSSERequest carries the caller's encryption headers into PutObject.
func WithSSERequest(ctx context.Context, algorithm, keyID string) context.Context {
	spec, _ := ctx.Value(sseCtxKey{}).(sseSpec)
	spec.Algorithm = strings.TrimSpace(algorithm)
	spec.KeyID = strings.TrimSpace(keyID)
	return context.WithValue(ctx, sseCtxKey{}, spec)
}

func withForcedEncryption(ctx context.Context, enc *metadata.ObjectEncryption) context.Context {
	spec, _ := ctx.Value(sseCtxKey{}).(sseSpec)
	spec.Forced = enc
	return context.WithValue(ctx, sseCtxKey{}, spec)
}

func sseSpecFrom(ctx context.Context) sseSpec {
	spec, _ := ctx.Value(sseCtxKey{}).(sseSpec)
	return spec
}

type byteCounter struct {
	r io.Reader
	n int64
}

func (b *byteCounter) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	b.n += int64(n)
	return n, err
}

// EnsureLocalKEK records the KEK fingerprint once and refuses a different key.
// The error text is part of the startup contract.
func EnsureLocalKEK(meta metadata.MetadataStore, master string) error {
	fp := kms.Fingerprint(master)
	n, _ := meta.CountObjects()
	st, err := meta.InitKMSState(metadata.KMSState{
		Fingerprint:      fp,
		LegacySSEObjects: fp != "" && n > 0,
	})
	if err != nil {
		return err
	}
	if st.Fingerprint == "" && fp != "" {
		st.Fingerprint = fp
		return meta.PutKMSState(st)
	}
	if st.Fingerprint != "" && fp != st.Fingerprint {
		return errors.New("SSE master key fingerprint mismatch")
	}
	return nil
}

func (s *Service) SetLocalKMS(local *kms.Local) {
	s.Local = local
}

func (s *Service) LocalKMSConfigured() bool {
	return s.Local != nil
}

func defaultKeyID(tenant, bucket string) string {
	clean := func(in string) string {
		in = strings.ToLower(strings.TrimSpace(in))
		if in == "" || in == "default" {
			return "default"
		}
		var b strings.Builder
		for _, r := range in {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				b.WriteRune(r)
			} else {
				b.WriteByte('-')
			}
		}
		out := strings.Trim(b.String(), "-")
		if out == "" {
			return "default"
		}
		return out
	}
	return "datasafe-" + clean(tenant) + "-" + clean(bucket)
}

func keyAllowed(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	raw := strings.TrimSpace(os.Getenv("STORAGE_KMS_KEY_ALLOWLIST"))
	if raw == "" {
		return strings.HasPrefix(id, "datasafe-")
	}
	for _, pattern := range strings.Split(raw, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if ok, _ := path.Match(pattern, id); ok {
			return true
		}
	}
	return false
}

func versionBelow160(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" || v == "dev" {
		return false
	}
	parts := strings.Split(v, ".")
	var nums [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return false
		}
		nums[i] = n
	}
	if nums[0] != 1 {
		return nums[0] < 1
	}
	return nums[1] < 6
}

func (s *Service) clusterUpgradeInProgress() bool {
	beats, err := s.Meta.ListNodeHeartbeats(time.Now().Add(-2 * time.Minute))
	if err != nil {
		return false
	}
	for _, b := range beats {
		if versionBelow160(b.Version) {
			return true
		}
	}
	return false
}

func (s *Service) legacySSE() bool {
	if s.Local == nil {
		return false
	}
	st, err := s.Meta.GetKMSState()
	return err == nil && st.LegacySSEObjects
}

func (s *Service) ensureKMSKey(id, tenant, bucketKey string) (metadata.KMSKeyRecord, error) {
	rec, err := s.Meta.GetKMSKey(id)
	if err == nil {
		if rec.Status == "destroyed" {
			return rec, ErrKeyDestroyed
		}
		return rec, nil
	}
	if !errors.Is(err, metadata.ErrNotFound) {
		return rec, err
	}
	rec = metadata.KMSKeyRecord{
		ID: id, Provider: kms.ProviderLocal, Status: "active",
		Version: 1, Generation: 1, TenantID: tenant, BucketKey: bucketKey,
	}
	if err := s.Meta.PutKMSKey(rec); err != nil {
		return rec, err
	}
	return rec, nil
}

// SetBucketEncryption writes the same bucket_encryption row the S3 API writes.
func (s *Service) SetBucketEncryption(rec metadata.BucketRecord, algo, keyID string) (metadata.BucketRecord, error) {
	if s.clusterUpgradeInProgress() {
		return rec, ErrClusterUpgrade
	}
	algo = strings.TrimSpace(algo)
	if algo != kms.SSEAlgorithmAES256 && algo != kms.SSEAlgorithmKMS {
		return rec, metadata.ErrInvalidArgument
	}
	if s.Local == nil {
		return rec, ErrSSEKeyMissing
	}
	if algo == kms.SSEAlgorithmKMS && rec.Visibility == "public-read" {
		return rec, ErrPublicKMS
	}
	if strings.TrimSpace(keyID) == "" {
		keyID = defaultKeyID(rec.TenantID, rec.Name)
	}
	if !keyAllowed(keyID) {
		return rec, ErrKeyNotAllowed
	}
	key, err := s.ensureKMSKey(keyID, rec.TenantID, rec.EffectiveStorageKey())
	if err != nil {
		return rec, err
	}
	rec.DefaultEncryption = algo
	rec.KMSMasterKeyID = key.ID
	rec.EncryptionProvider = kms.ProviderLocal
	rec.KMSKeyStatus = key.Status
	if err := s.Meta.UpdateBucket(rec); err != nil {
		return rec, err
	}
	err = s.Meta.PutBucketEncryption(rec.EffectiveStorageKey(), metadata.BucketEncryption{
		StorageKey: rec.EffectiveStorageKey(),
		Algorithm:  algo,
		KeyID:      key.ID,
		Provider:   kms.ProviderLocal,
		Status:     key.Status,
	})
	return rec, err
}

// ClearBucketEncryption removes the bucket default. Existing objects stay sealed.
func (s *Service) ClearBucketEncryption(rec metadata.BucketRecord) (metadata.BucketRecord, error) {
	if s.clusterUpgradeInProgress() {
		return rec, ErrClusterUpgrade
	}
	rec.DefaultEncryption = ""
	rec.KMSMasterKeyID = ""
	rec.EncryptionProvider = ""
	rec.KMSKeyStatus = ""
	if err := s.Meta.UpdateBucket(rec); err != nil {
		return rec, err
	}
	if err := s.Meta.DeleteBucketEncryption(rec.EffectiveStorageKey()); err != nil {
		return rec, err
	}
	return rec, nil
}

func (s *Service) resolveAlgo(ctx context.Context, bucket metadata.BucketRecord) (string, string) {
	spec := sseSpecFrom(ctx)
	if spec.Algorithm != "" {
		keyID := spec.KeyID
		if keyID == "" {
			keyID = bucket.KMSMasterKeyID
		}
		return spec.Algorithm, keyID
	}
	return bucket.DefaultEncryption, bucket.KMSMasterKeyID
}

func (s *Service) unwrapDEK(enc *metadata.ObjectEncryption) ([]byte, error) {
	if s.Local == nil {
		return nil, ErrSSEKeyMissing
	}
	if enc.Provider == kms.ProviderVault {
		return nil, ErrKMSUnavailable
	}
	raw, err := base64.StdEncoding.DecodeString(enc.WrappedDEK)
	if err != nil {
		return nil, err
	}
	ver := enc.KeyVersion
	if ver < 1 {
		ver = 1
	}
	return s.Local.Unwrap(context.Background(), enc.KeyID, ver, raw)
}

// prepareObjectEncryption decides how to seal the next write.
// The returned reader is what the backend stores. counter is non-nil when the
// plaintext length is known only after the reader is consumed.
func (s *Service) prepareObjectEncryption(ctx context.Context, bucket metadata.BucketRecord, r io.Reader) (io.Reader, *byteCounter, *metadata.ObjectEncryption, error) {
	spec := sseSpecFrom(ctx)
	if spec.Forced != nil && spec.Forced.Sealed() && spec.Forced.FormatVersion == 1 {
		dek, err := s.unwrapDEK(spec.Forced)
		if err != nil {
			return nil, nil, nil, err
		}
		c := &byteCounter{r: r}
		enc, err := kms.EncryptReader(dek, c, kms.DefaultChunk)
		if err != nil {
			return nil, nil, nil, err
		}
		meta := *spec.Forced
		return enc, c, &meta, nil
	}
	if s.clusterUpgradeInProgress() && s.Local != nil {
		plain, err := io.ReadAll(r)
		if err != nil {
			return nil, nil, nil, err
		}
		ct, err := s.Local.EncryptFormat0(plain)
		if err != nil {
			return nil, nil, nil, err
		}
		c := &byteCounter{n: int64(len(plain))}
		meta := &metadata.ObjectEncryption{
			FormatVersion: 0, Provider: kms.ProviderLocal, KeyVersion: 0,
			Alg: "AES-256-GCM", SSEAlgorithm: kms.SSEAlgorithmAES256, PlainSize: int64(len(plain)),
		}
		return bytes.NewReader(ct), c, meta, nil
	}
	algo, keyID := s.resolveAlgo(ctx, bucket)
	if algo == "" {
		return r, nil, &metadata.ObjectEncryption{Plaintext: true}, nil
	}
	if algo != kms.SSEAlgorithmAES256 && algo != kms.SSEAlgorithmKMS {
		return nil, nil, nil, metadata.ErrInvalidArgument
	}
	if algo == kms.SSEAlgorithmKMS && s.clusterUpgradeInProgress() {
		return nil, nil, nil, ErrClusterUpgrade
	}
	if s.Local == nil {
		return nil, nil, nil, ErrSSEKeyMissing
	}
	if keyID == "" {
		keyID = defaultKeyID(bucket.TenantID, bucket.Name)
	}
	if !keyAllowed(keyID) {
		return nil, nil, nil, ErrKeyNotAllowed
	}
	key, err := s.ensureKMSKey(keyID, bucket.TenantID, bucket.EffectiveStorageKey())
	if err != nil {
		return nil, nil, nil, err
	}
	dek, err := kms.NewDEK()
	if err != nil {
		return nil, nil, nil, err
	}
	ver := key.Version
	if ver < 1 {
		ver = 1
	}
	wrapped, ver, err := s.Local.WrapVersion(dek, ver)
	if err != nil {
		return nil, nil, nil, err
	}
	meta := &metadata.ObjectEncryption{
		FormatVersion: 1,
		Provider:      kms.ProviderLocal,
		KeyID:         key.ID,
		KeyVersion:    ver,
		WrappedDEK:    base64.StdEncoding.EncodeToString(wrapped),
		Alg:           "AES-256-GCM",
		SSEAlgorithm:  algo,
	}
	c := &byteCounter{r: r}
	enc, err := kms.EncryptReader(dek, c, kms.DefaultChunk)
	if err != nil {
		return nil, nil, nil, err
	}
	return enc, c, meta, nil
}

func (s *Service) openDecrypted(rc io.ReadCloser, rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	enc := rec.Encryption
	if enc == nil || !enc.Sealed() {
		if enc == nil && s.legacySSE() {
			return s.openFormat0(rc, rec)
		}
		return rc, rec, nil
	}
	if enc.KeyID != "" {
		key, err := s.Meta.GetKMSKey(enc.KeyID)
		if err == nil && key.Status == "destroyed" {
			rc.Close()
			return nil, rec, ErrKeyDestroyed
		}
	}
	if enc.Provider == kms.ProviderVault {
		rc.Close()
		return nil, rec, ErrKMSUnavailable
	}
	if enc.FormatVersion == 0 || enc.WrappedDEK == "" {
		return s.openFormat0(rc, rec)
	}
	dek, err := s.unwrapDEK(enc)
	if err != nil {
		rc.Close()
		return nil, rec, err
	}
	plain, err := kms.DecryptReader(dek, rc)
	if err != nil {
		rc.Close()
		return nil, rec, err
	}
	if enc.PlainSize > 0 {
		rec.Size = enc.PlainSize
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: plain, Closer: rc}, rec, nil
}

func (s *Service) openFormat0(rc io.ReadCloser, rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	defer rc.Close()
	if s.Local == nil {
		return nil, rec, ErrSSEKeyMissing
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, rec, err
	}
	plain, err := s.Local.DecryptFormat0(data)
	if err != nil {
		return nil, rec, err
	}
	rec.Size = int64(len(plain))
	return io.NopCloser(bytes.NewReader(plain)), rec, nil
}

type partOpener interface {
	OpenPart(ctx context.Context, bucket, key, uploadID string, partNumber int) (io.ReadCloser, error)
}

type closingReader struct {
	io.Reader
	c io.Closer
}

func (c closingReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	if err == io.EOF && c.c != nil {
		_ = c.c.Close()
		c.c = nil
	}
	return n, err
}

type partSequence struct {
	open  func(part int) (io.Reader, error)
	parts []storage.PartInfo
	i     int
	cur   io.Reader
}

func (q *partSequence) Read(p []byte) (int, error) {
	for {
		if q.cur == nil {
			if q.i >= len(q.parts) {
				return 0, io.EOF
			}
			r, err := q.open(q.parts[q.i].PartNumber)
			q.i++
			if err != nil {
				return 0, err
			}
			q.cur = r
		}
		n, err := q.cur.Read(p)
		if err == io.EOF {
			q.cur = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (s *Service) completeEncryptedMultipart(ctx context.Context, bucket, key, uploadID string, mp metadata.MultipartRecord, parts []storage.PartInfo, build func(metadata.ObjectRecord) []events.OutboxRecord) (metadata.ObjectRecord, int, error) {
	opener, ok := s.Backend.(partOpener)
	if !ok {
		return metadata.ObjectRecord{}, 0, errors.New("backend cannot read multipart parts")
	}
	dek, err := s.unwrapDEK(mp.Encryption)
	if err != nil {
		return metadata.ObjectRecord{}, 0, err
	}
	seq := &partSequence{
		parts: parts,
		open: func(part int) (io.Reader, error) {
			rc, err := opener.OpenPart(ctx, bucket, key, uploadID, part)
			if err != nil {
				return nil, err
			}
			plain, err := kms.DecryptReader(dek, rc)
			if err != nil {
				rc.Close()
				return nil, err
			}
			return closingReader{Reader: plain, c: rc}, nil
		},
	}
	rec, dropped, err := s.putObject(withForcedEncryption(ctx, mp.Encryption), bucket, key, seq, -1, "", nil, build)
	_ = s.Backend.AbortMultipartUpload(ctx, bucket, key, uploadID)
	if err != nil {
		return rec, dropped, err
	}
	_ = s.Meta.DeleteMultipart(uploadID)
	return rec, dropped, nil
}

func displayedSize(rec metadata.ObjectRecord, legacy bool) metadata.ObjectRecord {
	if rec.Encryption != nil && rec.Encryption.PlainSize > 0 {
		rec.Size = rec.Encryption.PlainSize
		return rec
	}
	if rec.Encryption != nil && rec.Encryption.FormatVersion == 0 && rec.Size > 28 {
		rec.Size -= 28
		return rec
	}
	if rec.Encryption == nil && legacy && rec.Size > 28 {
		rec.Size -= 28
	}
	return rec
}
