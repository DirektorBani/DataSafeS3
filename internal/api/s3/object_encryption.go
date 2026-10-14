package s3

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
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
	// MsgKMSDown is the console text when Vault cannot wrap or unwrap a DEK.
	MsgKMSDown = "Vault недоступен: новые файлы в этот бакет сейчас не сохранить, незашифрованные бакеты работают"
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

// legacyFormat0OptIn is the only way to treat unmarked objects as 1.5.0 SSE-S3.
// 1.5.0 rejected those writes (plaintext Content-Length versus ciphertext size),
// so a real directory does not contain them. Counting objects must not turn the
// flag on: that made every pre-existing plaintext GET fail, and the stored
// fingerprint then blocks starting again without the key.
func legacyFormat0OptIn() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_SSE_LEGACY_FORMAT0"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// EnsureLocalKEK records the KEK fingerprint once and refuses a different key.
// The error text is part of the startup contract.
// LegacySSEObjects follows STORAGE_SSE_LEGACY_FORMAT0 only. A flag stored by an
// earlier build that inferred it from the object count is cleared on this start.
// An empty master key does not create kms_state: the single-node gate runs
// without encryption, and a bolt write plus the heartbeat goroutine is enough
// to push idle RSS over the budget. A fingerprint already stored still refuses
// to start, so clearing the key cannot read objects wrapped by the old one.
func EnsureLocalKEK(meta metadata.MetadataStore, master string) error {
	master = strings.TrimSpace(master)
	if master == "" {
		st, err := meta.GetKMSState()
		if err != nil {
			return err
		}
		if st.Fingerprint != "" {
			return errors.New("SSE master key is missing; set STORAGE_SSE_MASTER_KEY to the original key and restart")
		}
		return nil
	}
	fp := kms.Fingerprint(master)
	wantLegacy := legacyFormat0OptIn()
	st, err := meta.InitKMSState(metadata.KMSState{
		Fingerprint:      fp,
		LegacySSEObjects: wantLegacy,
	})
	if err != nil {
		return err
	}
	if st.Fingerprint != "" && fp != st.Fingerprint {
		return errors.New("SSE master key fingerprint mismatch")
	}
	changed := false
	if st.Fingerprint == "" && fp != "" {
		st.Fingerprint = fp
		changed = true
	}
	if st.LegacySSEObjects != wantLegacy {
		st.LegacySSEObjects = wantLegacy
		changed = true
	}
	if changed {
		return meta.PutKMSState(st)
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
	if v := strings.TrimSpace(os.Getenv("STORAGE_KMS_DEFAULT_KEY")); v != "" {
		return v
	}
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

// HeartbeatVersion is the version this process writes for the upgrade gate.
// The encryption code in this binary is 1.6.0. The image build still stamps
// buildinfo.Version from the VERSION file (1.5.0) when the Docker arg is dev,
// and that stamp must not make the node refuse its own bucket-encryption change.
func HeartbeatVersion(build string) string {
	build = strings.TrimSpace(build)
	if build == "" || strings.EqualFold(build, "dev") || versionBelow160(build) {
		return "1.6.0"
	}
	return build
}

// EncryptionChangesBlocked is true while a 1.5.0 peer is still in the cluster.
func (s *Service) EncryptionChangesBlocked() bool {
	return s.clusterUpgradeInProgress()
}

func (s *Service) clusterUpgradeInProgress() bool {
	fresh := map[string]struct{}{}
	beats, err := s.Meta.ListNodeHeartbeats(time.Now().Add(-2 * time.Minute))
	if err == nil {
		for _, b := range beats {
			if versionBelow160(b.Version) {
				return true
			}
			fresh[b.NodeID] = struct{}{}
		}
	}
	// 1.5.0 does not write cluster_node_heartbeats. A live leader still renews
	// ha_leader_lock (lock_id storage-server, holder_id, acquired_at, expires_at).
	// A follower only retries that same INSERT. The WHERE clause does not match
	// while the lease is held, so the row is unchanged and the follower's
	// holder_id is not stored. There is no lease history. The migration advisory
	// lock is released after startup. pg_stat_activity shows the retry with an
	// empty application_name, the same as any other client, and the query text
	// has no version. That session is not a reliable 1.5.0 signal.
	if holder, ok, err := s.Meta.LiveLeader(); err == nil && ok && holder != "" {
		if _, seen := fresh[holder]; !seen {
			return true
		}
	}
	return s.peerPre160()
}

// peerPre160 reads version from GET /healthz of nodes in the cluster registry
// (system_config cluster.nodes, the list 1.5.0 E07 node status probes).
// A healthy 1.5.0 answer is {"status":"ok","version":"1.5.0",...}. An unreachable
// node is not live and does not block.
func (s *Service) peerPre160() bool {
	cfg, err := s.Meta.GetSystemConfig()
	if err != nil {
		return false
	}
	for _, n := range cfg.Cluster.Nodes {
		ver, alive := fetchPeerVersion(n.Address)
		if !alive {
			continue
		}
		if ver == "" || versionBelow160(ver) {
			return true
		}
	}
	return false
}

func fetchPeerVersion(address string) (string, bool) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", false
	}
	if !strings.Contains(address, "://") {
		address = "http://" + address
	}
	endpoint := strings.TrimRight(address, "/") + "/healthz"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", true
	}
	if body.Status != "" && body.Status != "ok" {
		return "", false
	}
	return body.Version, true
}

func (s *Service) legacySSE() bool {
	if s.Local == nil {
		return false
	}
	st, err := s.Meta.GetKMSState()
	return err == nil && st.LegacySSEObjects
}

// bucketKeyDestroyed is true after crypto-shred. Every object in the bucket
// is refused, including plaintext that was never wrapped.
func (s *Service) bucketKeyDestroyed(bucket string) bool {
	rec, err := s.Meta.GetBucketByKey(bucket)
	if err != nil {
		return false
	}
	if rec.KMSKeyStatus == "destroyed" {
		return true
	}
	if rec.KMSMasterKeyID == "" {
		return false
	}
	key, err := s.Meta.GetKMSKey(rec.KMSMasterKeyID)
	return err == nil && key.Status == "destroyed"
}

func (s *Service) ensureKMSKey(id, tenant, bucketKey, provider string) (metadata.KMSKeyRecord, error) {
	if provider == "" {
		provider = kms.ProviderLocal
	}
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
		ID: id, Provider: provider, Status: "active",
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
	if algo == kms.SSEAlgorithmKMS && rec.Visibility == "public-read" {
		return rec, ErrPublicKMS
	}
	provider := kms.ProviderLocal
	if algo == kms.SSEAlgorithmKMS && s.VaultKMS() != nil {
		provider = kms.ProviderVault
	}
	if provider == kms.ProviderLocal && s.Local == nil {
		return rec, ErrSSEKeyMissing
	}
	if strings.TrimSpace(keyID) == "" {
		keyID = defaultKeyID(rec.TenantID, rec.Name)
	}
	if !keyAllowed(keyID) {
		return rec, ErrKeyNotAllowed
	}
	key, err := s.ensureKMSKey(keyID, rec.TenantID, rec.EffectiveStorageKey(), provider)
	if err != nil {
		return rec, err
	}
	rec.DefaultEncryption = algo
	rec.KMSMasterKeyID = key.ID
	rec.EncryptionProvider = provider
	rec.KMSKeyStatus = key.Status
	if err := s.Meta.UpdateBucket(rec); err != nil {
		return rec, err
	}
	err = s.Meta.PutBucketEncryption(rec.EffectiveStorageKey(), metadata.BucketEncryption{
		StorageKey: rec.EffectiveStorageKey(),
		Algorithm:  algo,
		KeyID:      key.ID,
		Provider:   provider,
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
	if enc.Provider == kms.ProviderVault {
		v := s.VaultKMS()
		if v == nil {
			return nil, ErrKMSUnavailable
		}
		raw, err := base64.StdEncoding.DecodeString(enc.WrappedDEK)
		if err != nil {
			return nil, err
		}
		gen := int64(0)
		ver := enc.KeyVersion
		if ver < 1 {
			ver = 1
		}
		if key, err := s.Meta.GetKMSKey(enc.KeyID); err == nil {
			gen = key.Generation
		}
		dek, err := v.UnwrapDEK(context.Background(), enc.KeyID, ver, gen, raw)
		if err != nil {
			return nil, ErrKMSUnavailable
		}
		return dek, nil
	}
	if s.Local == nil {
		return nil, ErrSSEKeyMissing
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
	provider := kms.ProviderLocal
	vault := s.VaultKMS()
	if algo == kms.SSEAlgorithmKMS && vault != nil {
		provider = kms.ProviderVault
	}
	if provider == kms.ProviderLocal && s.Local == nil {
		return nil, nil, nil, ErrSSEKeyMissing
	}
	if keyID == "" {
		keyID = defaultKeyID(bucket.TenantID, bucket.Name)
	}
	if !keyAllowed(keyID) {
		return nil, nil, nil, ErrKeyNotAllowed
	}
	key, err := s.ensureKMSKey(keyID, bucket.TenantID, bucket.EffectiveStorageKey(), provider)
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
	var wrapped []byte
	if provider == kms.ProviderVault {
		wrapped, ver, err = vault.Wrap(ctx, key.ID, dek)
		if err != nil {
			return nil, nil, nil, ErrKMSUnavailable
		}
	} else {
		wrapped, ver, err = s.Local.WrapVersion(dek, ver)
		if err != nil {
			return nil, nil, nil, err
		}
	}
	meta := &metadata.ObjectEncryption{
		FormatVersion: 1,
		Provider:      provider,
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
		if enc != nil && enc.Plaintext {
			return rc, rec, nil
		}
		if s.legacySSE() {
			return s.openFormat0(rc, rec)
		}
		return s.openUnmarked(rc, rec)
	}
	if enc.KeyID != "" {
		key, err := s.Meta.GetKMSKey(enc.KeyID)
		if err == nil && key.Status == "destroyed" {
			rc.Close()
			return nil, rec, ErrKeyDestroyed
		}
	}
	if enc.Provider == kms.ProviderVault {
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

// openUnmarked serves an object that 1.5.0 stored with no encryption metadata.
// A format-1 body (DSSE, version 1) is not format 0 and is not returned as
// plaintext. Anything else is the original bytes: GCM is not attempted, because
// a failed authentication used to become HTTP 500 for every plaintext object.
func (s *Service) openUnmarked(rc io.ReadCloser, rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	var hdr [5]byte
	n, err := io.ReadFull(rc, hdr[:])
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return io.NopCloser(bytes.NewReader(hdr[:n])), rec, nil
	}
	if err != nil {
		rc.Close()
		return nil, rec, err
	}
	if bytes.Equal(hdr[:4], []byte{'D', 'S', 'S', 'E'}) && hdr[4] == 1 {
		rc.Close()
		return nil, rec, errors.New("object envelope is missing encryption metadata")
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(bytes.NewReader(hdr[:n]), rc), Closer: rc}, rec, nil
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
	rec, dropped, err := s.putObject(withForcedEncryption(ctx, mp.Encryption), bucket, key, seq, -1, mp.ContentType, mp.Metadata, build)
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
