package s3

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/observability"
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

	// format0MinLen is the shortest 1.5.0 blob: a 12-byte nonce and a 16-byte tag.
	format0MinLen = 12 + 16
	// sealAuthLimit is the plaintext size read fully before a success status.
	sealAuthLimit = 32 << 20
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

// legacyFormat0OptIn records STORAGE_SSE_LEGACY_FORMAT0. The flag does not
// decide which objects are ciphertext. Each unmarked object is classified
// once and the marker is what later reads use. Counting objects must not
// turn the flag on: a fingerprint stored from that guess blocks a later
// start with the key cleared.
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

// versionKnown160 is true when v parses as 1.6.0 or newer.
// Empty, dev, and unparseable strings are unknown. An unknown peer is old.
func versionKnown160(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if v == "" || strings.EqualFold(v, "dev") {
		return false
	}
	if versionBelow160(v) {
		return false
	}
	major, err := strconv.Atoi(strings.Split(v, ".")[0])
	return err == nil && major >= 1
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
	known := map[string]struct{}{}
	beats, err := s.Meta.ListNodeHeartbeats(time.Now().Add(-2 * time.Minute))
	if err == nil {
		for _, b := range beats {
			// RegisterNodeAddress can leave version blank. That row is an
			// address, not a peer version. A non-blank version that is not
			// 1.6.0 or newer is an old or unknown peer.
			if strings.TrimSpace(b.Version) == "" {
				continue
			}
			if !versionKnown160(b.Version) {
				return true
			}
			known[b.NodeID] = struct{}{}
		}
	}
	// 1.5.0 does not write cluster_node_heartbeats. A live leader still renews
	// ha_leader_lock. A follower retries the same INSERT. The ON CONFLICT WHERE
	// does not match while the lease is held, so the row stays with the leader.
	// Migration 030 records NEW.holder_id in ha_leader_attempts from a BEFORE
	// INSERT trigger, which runs even when that WHERE skips the update.
	// A recent attempt with no 1.6.0 heartbeat has an unknown version and is
	// treated as 1.5.0. A stopped node leaves the two-minute window.
	if holder, ok, err := s.Meta.LiveLeader(); err == nil && ok && holder != "" {
		if _, seen := known[holder]; !seen {
			return true
		}
	}
	if attempts, err := s.Meta.ListRecentLeaderAttempts(time.Now().Add(-2 * time.Minute)); err == nil {
		for _, id := range attempts {
			if _, seen := known[id]; !seen {
				return true
			}
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

// ProbeSealedObject reports whether a share page can offer this object.
// SSE-KMS whose data key cannot be unwrapped is unavailable. A destroyed key
// is refused. Plaintext and SSE-S3 do not call Vault.
func (s *Service) ProbeSealedObject(rec metadata.ObjectRecord) error {
	enc := rec.Encryption
	if enc == nil || !enc.Sealed() {
		return nil
	}
	if enc.KeyID != "" {
		key, err := s.Meta.GetKMSKey(enc.KeyID)
		if err == nil && key.Status == "destroyed" {
			return ErrKeyDestroyed
		}
	}
	if enc.Provider != kms.ProviderVault {
		return nil
	}
	_, err := s.unwrapDEK(enc)
	return err
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
	algo, keyID := s.resolveAlgo(ctx, bucket)
	if algo == "" {
		return r, nil, &metadata.ObjectEncryption{Plaintext: true}, nil
	}
	if algo != kms.SSEAlgorithmAES256 && algo != kms.SSEAlgorithmKMS {
		return nil, nil, nil, metadata.ErrInvalidArgument
	}
	// A 1.5.0 peer cannot read format 1. SSE-S3 requested during the roll is
	// format 0, which that peer can read when it has the same server key.
	// A write with no encryption header stays plaintext. SSE-KMS stays 409.
	if s.clusterUpgradeInProgress() && s.Local != nil && algo == kms.SSEAlgorithmAES256 {
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
		return finishSealed(rc, plain, rec)
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
	return finishSealed(rc, plain, rec)
}

// finishSealed authenticates ciphertext before the caller writes HTTP 200.
// Up to sealAuthLimit the whole object is opened in memory. A larger object
// is checked on the first byte, which is the first chunk, and then streamed.
func finishSealed(rc io.ReadCloser, plain io.Reader, rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	if rec.Size <= sealAuthLimit {
		data, err := io.ReadAll(plain)
		_ = rc.Close()
		if err != nil {
			return nil, rec, err
		}
		rec.Size = int64(len(data))
		return io.NopCloser(bytes.NewReader(data)), rec, nil
	}
	var one [1]byte
	n, err := plain.Read(one[:])
	if err != nil && err != io.EOF {
		_ = rc.Close()
		return nil, rec, err
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(bytes.NewReader(one[:n]), plain), Closer: rc}, rec, nil
}

// openUnmarked serves an object 1.5.0 stored with no encryption metadata.
// A recorded size below the format-0 minimum cannot be ciphertext and is
// streamed. Anything longer is classified once: a blob that authenticates is
// marked format 0 and then decrypted, and every other body is marked plaintext
// and streamed. Reads after that use the marker and do not try again.
func (s *Service) openUnmarked(rc io.ReadCloser, rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	if s.Local == nil {
		return s.openUnmarkedRaw(rc, rec)
	}
	if rec.Size >= 0 && rec.Size < format0MinLen {
		_ = rc.Close()
		s.persistLegacy(rec, plaintextMarker())
		noteLegacyClassify("plaintext", rec)
		return s.reopenPlain(rec)
	}
	noteLegacyClassifyRead(rec)
	ok, err := s.Local.AuthenticateFormat0(rc)
	observability.SetLegacyClassifyCurrent(0)
	_ = rc.Close()
	if err != nil {
		noteLegacyClassify("error", rec)
		return nil, rec, err
	}
	if ok {
		marker := format0Marker(rec.Size - format0MinLen)
		s.persistLegacy(rec, marker)
		noteLegacyClassify("format0", rec)
		return s.reopenFormat0(rec)
	}
	s.persistLegacy(rec, plaintextMarker())
	noteLegacyClassify("plaintext", rec)
	return s.reopenPlain(rec)
}

func plaintextMarker() *metadata.ObjectEncryption {
	return &metadata.ObjectEncryption{Plaintext: true}
}

func format0Marker(plainSize int64) *metadata.ObjectEncryption {
	if plainSize < 0 {
		plainSize = 0
	}
	return &metadata.ObjectEncryption{
		FormatVersion: 0,
		Provider:      kms.ProviderLocal,
		Alg:           "AES-256-GCM",
		SSEAlgorithm:  kms.SSEAlgorithmAES256,
		PlainSize:     plainSize,
	}
}

func legacyClassified(enc *metadata.ObjectEncryption) bool {
	return enc != nil && (enc.Plaintext || enc.Sealed())
}

func (s *Service) persistLegacy(rec metadata.ObjectRecord, marker *metadata.ObjectEncryption) {
	if s == nil || s.Meta == nil || marker == nil {
		return
	}
	if _, err := s.Meta.SetObjectEncryptionIfAbsent(rec.Bucket, rec.Key, rec.VersionID, marker); err != nil {
		slog.Warn("legacy object classification was not stored", "bucket", rec.Bucket, "key", rec.Key, "err", err)
	}
}

func (s *Service) reopenPlain(rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	rc, _, err := s.Backend.GetObjectVersion(context.Background(), rec.Bucket, rec.Key, rec.VersionID)
	if err != nil {
		return nil, rec, err
	}
	rec.Encryption = plaintextMarker()
	return rc, rec, nil
}

func (s *Service) reopenFormat0(rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	rc, _, err := s.Backend.GetObjectVersion(context.Background(), rec.Bucket, rec.Key, rec.VersionID)
	if err != nil {
		return nil, rec, err
	}
	rec.Encryption = format0Marker(rec.Size - format0MinLen)
	return s.openFormat0(rc, rec)
}

// RunLegacyClassify walks unmarked objects once and stores a format marker.
// It is resumable: a marker already stored is skipped, and a restart continues
// with whatever is still unmarked. Bolt and Postgres both accept the same
// conditional write, so two HA nodes classifying one object keep one result.
func (s *Service) RunLegacyClassify(ctx context.Context) {
	if s == nil || s.Local == nil || s.Meta == nil || s.Backend == nil {
		return
	}
	buckets, err := s.Meta.ListBuckets()
	if err != nil {
		slog.Warn("legacy classification could not list buckets", "err", err)
		return
	}
	slog.Info("legacy classification started")
	var marked, skipped, failed int
	defer func() {
		slog.Info("legacy classification finished", "marked", marked, "skipped", skipped, "errors", failed)
	}()
	for _, b := range buckets {
		if ctx.Err() != nil {
			return
		}
		sk := b.EffectiveStorageKey()
		start := ""
		for {
			if ctx.Err() != nil {
				return
			}
			page, trunc, next, err := s.Meta.ListObjectsPage(sk, "", start, 200)
			if err != nil {
				slog.Warn("legacy classification could not list objects", "bucket", sk, "err", err)
				break
			}
			for _, rec := range page {
				if legacyClassified(rec.Encryption) || rec.IsDeleteMarker {
					skipped++
					noteLegacyClassify("skipped", rec)
					continue
				}
				result, err := s.classifyStored(ctx, rec)
				if err != nil {
					failed++
					noteLegacyClassify("error", rec)
					slog.Warn("legacy classification skipped an object", "bucket", rec.Bucket, "key", rec.Key, "err", err)
					continue
				}
				marked++
				noteLegacyClassify(result, rec)
			}
			seen := marked + skipped + failed
			if seen > 0 && seen%100 == 0 {
				slog.Info("legacy classification progress", "marked", marked, "skipped", skipped, "errors", failed)
			}
			if !trunc || next == "" || next == start {
				break
			}
			start = next
		}
	}
}

func (s *Service) classifyStored(ctx context.Context, rec metadata.ObjectRecord) (string, error) {
	if rec.Size >= 0 && rec.Size < format0MinLen {
		s.persistLegacy(rec, plaintextMarker())
		return "plaintext", nil
	}
	noteLegacyClassifyRead(rec)
	defer observability.SetLegacyClassifyCurrent(0)
	rc, _, err := s.Backend.GetObjectVersion(ctx, rec.Bucket, rec.Key, rec.VersionID)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	ok, err := s.Local.AuthenticateFormat0(rc)
	if err != nil {
		return "", err
	}
	if ok {
		s.persistLegacy(rec, format0Marker(rec.Size-format0MinLen))
		return "format0", nil
	}
	s.persistLegacy(rec, plaintextMarker())
	return "plaintext", nil
}

// legacyClassifyLogBytes is the size at which one object is logged on its own.
// Smaller objects are counted in the periodic progress line.
const legacyClassifyLogBytes = 32 << 20

func noteLegacyClassifyRead(rec metadata.ObjectRecord) {
	if rec.Size < legacyClassifyLogBytes {
		return
	}
	slog.Info("legacy classification reading object", "bucket", rec.Bucket, "key", rec.Key, "bytes", rec.Size)
	observability.SetLegacyClassifyCurrent(rec.Size)
}

func noteLegacyClassify(result string, rec metadata.ObjectRecord) {
	observability.ObserveLegacyClassify(result)
	if rec.Size >= legacyClassifyLogBytes {
		slog.Info("legacy classification finished object", "bucket", rec.Bucket, "key", rec.Key, "bytes", rec.Size, "result", result)
	}
}

func (s *Service) openUnmarkedRaw(rc io.ReadCloser, rec metadata.ObjectRecord) (io.ReadCloser, metadata.ObjectRecord, error) {
	var hdr [5]byte
	n, err := io.ReadFull(rc, hdr[:])
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return io.NopCloser(bytes.NewReader(hdr[:n])), rec, nil
	}
	if err != nil {
		rc.Close()
		return nil, rec, err
	}
	if unmarkedDSSE(hdr[:n]) {
		rc.Close()
		return nil, rec, errors.New("object envelope is missing encryption metadata")
	}
	return struct {
		io.Reader
		io.Closer
	}{Reader: io.MultiReader(bytes.NewReader(hdr[:n]), rc), Closer: rc}, rec, nil
}

func unmarkedDSSE(b []byte) bool {
	return len(b) >= 5 && bytes.Equal(b[:4], []byte{'D', 'S', 'S', 'E'}) && b[4] == 1
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

func displayedSize(rec metadata.ObjectRecord) metadata.ObjectRecord {
	if rec.Encryption != nil && rec.Encryption.Plaintext {
		return rec
	}
	if rec.Encryption != nil && rec.Encryption.PlainSize > 0 {
		rec.Size = rec.Encryption.PlainSize
		return rec
	}
	if rec.Encryption != nil && rec.Encryption.Sealed() && rec.Encryption.FormatVersion == 0 && rec.Size >= format0MinLen {
		rec.Size -= format0MinLen
	}
	return rec
}
