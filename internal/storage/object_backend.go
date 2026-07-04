package storage

import (
	"context"
	"io"
	"os"
	"strings"
	"time"
)

// BackendHealth reports object backend shard health.
type BackendHealth struct {
	Degraded        bool
	DegradedSets    int
	HealJobsPending int
}

// HealthReporter is implemented by backends that expose shard health.
type HealthReporter interface {
	Health(ctx context.Context) BackendHealth
}

// ObjectBackend is the storage contract used by the S3 service layer.
type ObjectBackend interface {
	Backend
	PutObjectVersion(ctx context.Context, bucket, key, versionID string, r io.Reader, size int64, contentType string) (etag string, err error)
	GetObjectVersion(ctx context.Context, bucket, key, versionID string) (rc io.ReadCloser, info ObjectInfo, err error)
	StatObjectVersion(ctx context.Context, bucket, key, versionID string) (ObjectInfo, error)
	DeleteObjectVersion(ctx context.Context, bucket, key, versionID string) error
	DeleteAllObjectVersions(ctx context.Context, bucket, key string) error
	CreateBucket(bucket string) error
	DeleteBucket(bucket string) error
	BucketExists(bucket string) bool
	ListBuckets() ([]string, error)
	ListObjectKeys(bucket, prefix string) ([]ObjectInfo, error)
}

// ObjectBackendConfig selects fs vs erasure object storage.
type ObjectBackendConfig struct {
	DataDir   string
	ReadOnly  bool
	Backend   string // fs|erasure
	Layout    string // dev|production
	Paths     []string
	HealEvery time.Duration
}

// ObjectBackendFromEnv builds config from environment variables.
func ObjectBackendFromEnv(dataDir string, readOnly bool) ObjectBackendConfig {
	cfg := ObjectBackendConfig{
		DataDir:  dataDir,
		ReadOnly: readOnly,
		Backend:  strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_OBJECT_BACKEND"))),
		Layout:   strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_ERASURE_LAYOUT"))),
	}
	if cfg.Backend == "" {
		cfg.Backend = "fs"
	}
	if cfg.Layout == "" {
		cfg.Layout = "dev"
	}
	if raw := strings.TrimSpace(os.Getenv("STORAGE_ERASURE_DATA_PATHS")); raw != "" {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if idx := strings.Index(p, ":"); idx > 0 && !strings.Contains(p, `\`) && !strings.Contains(p[1:idx], `\`) {
				// nodeId:path — use path portion only for local dev.
				p = strings.TrimSpace(p[idx+1:])
			}
			cfg.Paths = append(cfg.Paths, p)
		}
	}
	heal := 5 * time.Minute
	if v := strings.TrimSpace(os.Getenv("STORAGE_ERASURE_HEAL_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			heal = d
		}
	}
	cfg.HealEvery = heal
	return cfg
}

// HealthOf returns backend health when supported.
func HealthOf(ctx context.Context, b ObjectBackend) BackendHealth {
	if hr, ok := b.(HealthReporter); ok {
		return hr.Health(ctx)
	}
	return BackendHealth{}
}
