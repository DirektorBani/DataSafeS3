package api

import (
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Server) ensureDailySnapshot() {
	today := metadata.UsageSnapshot{
		Date: time.Now().UTC().Format("2006-01-02"),
	}
	buckets, _ := s.meta.ListBuckets()
	today.BucketCount = len(buckets)
	bytes, _ := s.meta.TotalObjectBytes()
	today.StorageBytes = bytes
	objs, _ := s.meta.CountObjects()
	today.ObjectCount = objs
	_ = s.meta.PutUsageSnapshot(today)
}
