package api

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

type homeBucketConfig struct {
	Enabled bool
	Name    string
}

func homeBucketConfigFromEnv() homeBucketConfig {
	enabled := true
	if v := strings.TrimSpace(os.Getenv("STORAGE_AUTO_HOME_BUCKET")); v != "" {
		switch strings.ToLower(v) {
		case "0", "false", "no", "off":
			enabled = false
		}
	}
	name := strings.TrimSpace(os.Getenv("STORAGE_HOME_BUCKET_NAME"))
	if name == "" {
		name = "files"
	}
	return homeBucketConfig{Enabled: enabled, Name: name}
}

func homeBucketQuotaFromEnv() (maxSize int64, maxObjects int64) {
	if v := strings.TrimSpace(os.Getenv("STORAGE_HOME_BUCKET_MAX_SIZE_BYTES")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxSize = n
		}
	} else {
		maxSize = 10 * 1024 * 1024 * 1024 // 10 GiB default
	}
	if v := strings.TrimSpace(os.Getenv("STORAGE_HOME_BUCKET_MAX_OBJECTS")); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			maxObjects = n
		}
	}
	return maxSize, maxObjects
}

// countOwnedBucketsInOwnerScope counts personal buckets owned by the user (ScopeOwner, non-tenant).
func (s *Server) countOwnedBucketsInOwnerScope(userID, username string) int {
	filter := metadata.BucketListFilter{UserID: userID, Username: username}
	buckets, err := s.meta.ListBucketsFiltered(filter)
	if err != nil {
		return 0
	}
	n := 0
	for _, b := range buckets {
		if b.OwnerID != userID && b.Owner != username {
			continue
		}
		if metadata.IsTenantScoped(b.EffectiveTenantID()) {
			continue
		}
		n++
	}
	return n
}

// ensureHomeBucket provisions the default personal bucket when auto-create is enabled and the user has none.
func (s *Server) ensureHomeBucket(info auth.TokenInfo) {
	cfg := homeBucketConfigFromEnv()
	if !cfg.Enabled || info.UserID == "" {
		return
	}
	if s.countOwnedBucketsInOwnerScope(info.UserID, info.Username) > 0 {
		return
	}
	scope := metadata.BucketScope{Kind: metadata.ScopeOwner, OwnerID: info.UserID}
	if _, err := s.meta.ResolveBucket(scope, cfg.Name); err == nil {
		return
	}
	if err := s.svc.CreateBucket(context.Background(), cfg.Name, info.Username); err != nil {
		return
	}
	s.stampBucketOwnership(cfg.Name, info)
	_ = s.applyBucketVisibilityPolicy(info, cfg.Name, "private")
	maxSize, maxObjects := homeBucketQuotaFromEnv()
	if maxSize > 0 || maxObjects > 0 {
		if rec, err := s.resolveBucketForUser(info, cfg.Name); err == nil {
			if maxSize > 0 {
				rec.MaxSizeBytes = maxSize
			}
			if maxObjects > 0 {
				rec.MaxObjects = maxObjects
			}
			_ = s.meta.UpdateBucket(rec)
		}
	}
}
