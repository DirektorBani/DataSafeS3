package erasure

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/storage"
)

// Config configures an erasure object backend.
type Config struct {
	Paths     []string
	Layout    string
	ReadOnly  bool
	Staging   string // FS staging dir for buckets/multipart
	HealEvery time.Duration
}

type objectMeta struct {
	OrigLen     int    `json:"orig_len"`
	ContentType string `json:"content_type,omitempty"`
}

// Backend stores object bytes as erasure-coded shards across paths.
type Backend struct {
	paths    []string
	codec    ShardCodec
	layout   Layout
	staging  *storage.FSBackend
	readOnly bool
	mu       sync.RWMutex
	health   storage.BackendHealth
}

// OpenBackend creates an erasure backend with optional heal worker registration.
func OpenBackend(cfg Config) (*Backend, error) {
	if len(cfg.Paths) < 1 {
		return nil, fmt.Errorf("erasure: STORAGE_ERASURE_DATA_PATHS required")
	}
	codec, err := NewCodec(cfg.Layout)
	if err != nil {
		return nil, err
	}
	layout := codec.Layout()
	if len(cfg.Paths) < layout.total() {
		return nil, fmt.Errorf("erasure: need at least %d paths, got %d", layout.total(), len(cfg.Paths))
	}
	for _, p := range cfg.Paths {
		if cfg.ReadOnly {
			if _, err := os.Stat(p); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
		} else if err := os.MkdirAll(p, 0o755); err != nil {
			return nil, err
		}
	}
	var staging *storage.FSBackend
	var err2 error
	if cfg.ReadOnly {
		staging, err2 = storage.OpenFSBackend(cfg.Staging)
	} else {
		staging, err2 = storage.NewFSBackend(cfg.Staging)
	}
	if err2 != nil {
		return nil, err2
	}
	b := &Backend{
		paths:    cfg.Paths,
		codec:    codec,
		layout:   layout,
		staging:  staging,
		readOnly: cfg.ReadOnly,
	}
	b.refreshHealth(context.Background())
	return b, nil
}

func (b *Backend) shardDir(bucket, key, versionID string) string {
	rel := storageKeyPath(bucket, key, versionID)
	return rel
}

func storageKeyPath(bucket, key, versionID string) string {
	key = strings.TrimPrefix(key, "/")
	if key == "" {
		key = "_empty"
	}
	rel := filepath.Join(bucket, filepath.FromSlash(key))
	if versionID != "" {
		rel = filepath.Join(rel, "versions", versionID)
	}
	return rel
}

func (b *Backend) shardPath(pathIdx int, bucket, key, versionID string, shard int) string {
	return filepath.Join(b.paths[pathIdx], b.shardDir(bucket, key, versionID), fmt.Sprintf("shard-%d.bin", shard))
}

func (b *Backend) metaPath(pathIdx int, bucket, key, versionID string) string {
	return filepath.Join(b.paths[pathIdx], b.shardDir(bucket, key, versionID), "meta.json")
}

// writeMeta stores object metadata on every erasure path so losing paths[0] alone
// cannot make a recoverable object unreadable.
func (b *Backend) writeMeta(bucket, key, versionID string, meta objectMeta) error {
	if b.readOnly {
		return fmt.Errorf("read-only")
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	var firstErr error
	ok := 0
	for i := range b.paths {
		path := b.metaPath(i, bucket, key, versionID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, data, 0o644); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := os.Rename(tmp, path); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ok++
	}
	// Quorum: at least one full copy beyond a single disk — prefer majority of paths.
	need := (len(b.paths) + 1) / 2
	if need < 1 {
		need = 1
	}
	if ok < need {
		if firstErr != nil {
			return fmt.Errorf("erasure meta write: %d/%d paths ok: %w", ok, len(b.paths), firstErr)
		}
		return fmt.Errorf("erasure meta write: %d/%d paths ok", ok, len(b.paths))
	}
	return nil
}

func (b *Backend) readMeta(bucket, key, versionID string) (objectMeta, error) {
	var lastErr error
	for i := range b.paths {
		data, err := os.ReadFile(b.metaPath(i, bucket, key, versionID))
		if err != nil {
			if os.IsNotExist(err) {
				lastErr = storage.ErrNotFound
				continue
			}
			lastErr = err
			continue
		}
		var meta objectMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			lastErr = err
			continue
		}
		return meta, nil
	}
	if lastErr != nil {
		return objectMeta{}, lastErr
	}
	return objectMeta{}, storage.ErrNotFound
}

func (b *Backend) metaModTime(bucket, key, versionID string) time.Time {
	for i := range b.paths {
		st, err := os.Stat(b.metaPath(i, bucket, key, versionID))
		if err == nil {
			return st.ModTime().UTC()
		}
	}
	return time.Now().UTC()
}

func (b *Backend) removeAllMeta(bucket, key, versionID string) {
	for i := range b.paths {
		path := b.metaPath(i, bucket, key, versionID)
		_ = os.Remove(path)
		_ = os.Remove(filepath.Dir(path))
	}
}

func (b *Backend) PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (string, error) {
	return b.PutObjectVersion(ctx, bucket, key, "", r, size, contentType)
}

func (b *Backend) PutObjectVersion(_ context.Context, bucket, key, versionID string, r io.Reader, size int64, contentType string) (string, error) {
	if b.readOnly {
		return "", fmt.Errorf("read-only")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if size >= 0 && int64(len(data)) != size {
		return "", fmt.Errorf("size mismatch: expected %d got %d", size, len(data))
	}
	etag := `"` + hex.EncodeToString(md5Sum(data)) + `"`
	shards, err := b.codec.Encode(data)
	if err != nil {
		return "", err
	}
	for i, shard := range shards {
		path := b.shardPath(i, bucket, key, versionID, i)
		if err := writeShard(path, shard); err != nil {
			return "", err
		}
	}
	if err := b.writeMeta(bucket, key, versionID, objectMeta{OrigLen: len(data), ContentType: contentType}); err != nil {
		return "", err
	}
	b.refreshHealth(context.Background())
	return etag, nil
}

func md5Sum(data []byte) []byte {
	h := md5.New()
	_, _ = h.Write(data)
	return h.Sum(nil)
}

func writeShard(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (b *Backend) readShards(bucket, key, versionID string) ([][]byte, objectMeta, error) {
	meta, err := b.readMeta(bucket, key, versionID)
	if err != nil {
		return nil, objectMeta{}, err
	}
	total := b.layout.total()
	shards := make([][]byte, total)
	for i := 0; i < total; i++ {
		path := b.shardPath(i, bucket, key, versionID, i)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				shards[i] = nil
				continue
			}
			return nil, objectMeta{}, err
		}
		shards[i] = data
	}
	return shards, meta, nil
}

func (b *Backend) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, storage.ObjectInfo, error) {
	return b.GetObjectVersion(ctx, bucket, key, "")
}

func (b *Backend) GetObjectVersion(_ context.Context, bucket, key, versionID string) (io.ReadCloser, storage.ObjectInfo, error) {
	shards, meta, err := b.readShards(bucket, key, versionID)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	data, err := b.codec.Decode(shards, meta.OrigLen)
	if err != nil {
		return nil, storage.ObjectInfo{}, err
	}
	etag := `"` + hex.EncodeToString(md5Sum(data)) + `"`
	info := storage.ObjectInfo{
		Bucket:       bucket,
		Key:          key,
		Size:         int64(len(data)),
		ETag:         etag,
		ContentType:  meta.ContentType,
		LastModified: b.metaModTime(bucket, key, versionID),
	}
	return io.NopCloser(strings.NewReader(string(data))), info, nil
}

func fileModTime(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Now().UTC()
	}
	return st.ModTime().UTC()
}

func (b *Backend) StatObject(ctx context.Context, bucket, key string) (storage.ObjectInfo, error) {
	return b.StatObjectVersion(ctx, bucket, key, "")
}

func (b *Backend) StatObjectVersion(_ context.Context, bucket, key, versionID string) (storage.ObjectInfo, error) {
	meta, err := b.readMeta(bucket, key, versionID)
	if err != nil {
		return storage.ObjectInfo{}, err
	}
	etagBytes, _ := b.peekETag(bucket, key, versionID, meta.OrigLen)
	etag := `"` + hex.EncodeToString(etagBytes) + `"`
	return storage.ObjectInfo{
		Bucket:       bucket,
		Key:          key,
		Size:         int64(meta.OrigLen),
		ETag:         etag,
		ContentType:  meta.ContentType,
		LastModified: b.metaModTime(bucket, key, versionID),
	}, nil
}

func (b *Backend) peekETag(bucket, key, versionID string, origLen int) ([]byte, error) {
	shards, _, err := b.readShards(bucket, key, versionID)
	if err != nil {
		return nil, err
	}
	data, err := b.codec.Decode(shards, origLen)
	if err != nil {
		return nil, err
	}
	return md5Sum(data), nil
}

func (b *Backend) DeleteObject(ctx context.Context, bucket, key string) error {
	return b.DeleteObjectVersion(ctx, bucket, key, "")
}

func (b *Backend) DeleteObjectVersion(_ context.Context, bucket, key, versionID string) error {
	if b.readOnly {
		return fmt.Errorf("read-only")
	}
	total := b.layout.total()
	for i := 0; i < total; i++ {
		_ = os.Remove(b.shardPath(i, bucket, key, versionID, i))
	}
	b.removeAllMeta(bucket, key, versionID)
	b.refreshHealth(context.Background())
	return nil
}

func (b *Backend) DeleteAllObjectVersions(_ context.Context, bucket, key string) error {
	return b.DeleteObject(context.Background(), bucket, key)
}

// Multipart and bucket ops delegate to staging FS backend.
func (b *Backend) CreateMultipartUpload(ctx context.Context, bucket, key, uploadID string) error {
	return b.staging.CreateMultipartUpload(ctx, bucket, key, uploadID)
}
func (b *Backend) UploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, r io.Reader, size int64) (string, error) {
	return b.staging.UploadPart(ctx, bucket, key, uploadID, partNumber, r, size)
}
func (b *Backend) ListParts(ctx context.Context, bucket, key, uploadID string) ([]storage.PartInfo, error) {
	return b.staging.ListParts(ctx, bucket, key, uploadID)
}
func (b *Backend) CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []storage.PartInfo) (string, error) {
	_, err := b.staging.CompleteMultipartUpload(ctx, bucket, key, uploadID, parts)
	if err != nil {
		return "", err
	}
	rc, info, err := b.staging.GetObject(ctx, bucket, key)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	etag2, err := b.PutObject(ctx, bucket, key, rc, info.Size, info.ContentType)
	if err != nil {
		return "", err
	}
	_ = b.staging.DeleteObject(ctx, bucket, key)
	return etag2, nil
}
func (b *Backend) AbortMultipartUpload(ctx context.Context, bucket, key, uploadID string) error {
	return b.staging.AbortMultipartUpload(ctx, bucket, key, uploadID)
}
func (b *Backend) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (string, error) {
	rc, info, err := b.GetObject(ctx, srcBucket, srcKey)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	return b.PutObject(ctx, dstBucket, dstKey, rc, info.Size, info.ContentType)
}
func (b *Backend) CreateBucket(bucket string) error { return b.staging.CreateBucket(bucket) }
func (b *Backend) DeleteBucket(bucket string) error { return b.staging.DeleteBucket(bucket) }
func (b *Backend) BucketExists(bucket string) bool  { return b.staging.BucketExists(bucket) }
func (b *Backend) ListBuckets() ([]string, error)   { return b.staging.ListBuckets() }
func (b *Backend) ListObjectKeys(bucket, prefix string) ([]storage.ObjectInfo, error) {
	return b.staging.ListObjectKeys(bucket, prefix)
}

func (b *Backend) Health(ctx context.Context) storage.BackendHealth {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.health
}

func (b *Backend) refreshHealth(_ context.Context) {
	degraded := 0
	seen := map[string]struct{}{}
	for _, root := range b.paths {
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != "meta.json" {
				return nil
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil || rel == "." {
				return nil
			}
			if _, ok := seen[rel]; ok {
				return nil
			}
			seen[rel] = struct{}{}
			missing := 0
			for i := 0; i < b.layout.total(); i++ {
				sp := filepath.Join(b.paths[i], rel, fmt.Sprintf("shard-%d.bin", i))
				if _, err := os.Stat(sp); err != nil {
					missing++
				}
			}
			if missing > 0 && missing <= b.layout.ParityShards {
				degraded++
			}
			return nil
		})
	}
	h := storage.BackendHealth{Degraded: degraded > 0, DegradedSets: degraded}
	b.mu.Lock()
	b.health = h
	b.mu.Unlock()
	observability.SetErasureDegradedSets(degraded)
}

// HealOnce rebuilds missing shards (and missing meta replicas) for known objects.
func (b *Backend) HealOnce(ctx context.Context) (int64, error) {
	if b.readOnly {
		return 0, nil
	}
	var healedBytes int64
	seen := map[string]struct{}{}
	for _, root := range b.paths {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || d.Name() != "meta.json" {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil || rel == "." {
				return nil
			}
			if _, ok := seen[rel]; ok {
				return nil
			}
			seen[rel] = struct{}{}

			metaBytes, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			var meta objectMeta
			if json.Unmarshal(metaBytes, &meta) != nil {
				return nil
			}
			// Replicate meta to any path that lost it.
			for i := range b.paths {
				mp := filepath.Join(b.paths[i], rel, "meta.json")
				if _, err := os.Stat(mp); err == nil {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(mp), 0o755); err != nil {
					continue
				}
				tmp := mp + ".tmp"
				if err := os.WriteFile(tmp, metaBytes, 0o644); err != nil {
					continue
				}
				if err := os.Rename(tmp, mp); err != nil {
					continue
				}
				healedBytes += int64(len(metaBytes))
			}

			total := b.layout.total()
			shards := make([][]byte, total)
			missing := false
			for i := 0; i < total; i++ {
				sp := filepath.Join(b.paths[i], rel, fmt.Sprintf("shard-%d.bin", i))
				data, err := os.ReadFile(sp)
				if err != nil {
					shards[i] = nil
					missing = true
					continue
				}
				shards[i] = data
			}
			if !missing {
				return nil
			}
			data, err := b.codec.Decode(shards, meta.OrigLen)
			if err != nil {
				return nil
			}
			newShards, err := b.codec.Encode(data)
			if err != nil {
				return nil
			}
			for i, shard := range newShards {
				sp := filepath.Join(b.paths[i], rel, fmt.Sprintf("shard-%d.bin", i))
				if _, err := os.Stat(sp); err == nil {
					continue
				}
				if err := writeShard(sp, shard); err != nil {
					return nil
				}
				healedBytes += int64(len(shard))
			}
			return nil
		})
		if err != nil {
			b.refreshHealth(ctx)
			observability.AddErasureHealBytes(healedBytes)
			return healedBytes, err
		}
	}
	b.refreshHealth(ctx)
	observability.AddErasureHealBytes(healedBytes)
	return healedBytes, nil
}

var _ storage.ObjectBackend = (*Backend)(nil)
