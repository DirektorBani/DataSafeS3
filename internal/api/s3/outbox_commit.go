package s3

import (
	"context"
	"log/slog"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Service) commitObjectRecord(rec metadata.ObjectRecord, versioned bool, evs []events.OutboxRecord) (int, error) {
	if len(evs) == 0 {
		if versioned {
			return 0, s.Meta.PutObjectVersioned(rec)
		}
		return 0, s.Meta.PutObject(rec)
	}
	if w, ok := s.Meta.(metadata.OutboxMutator); ok {
		if versioned {
			return w.PutObjectVersionedWithOutbox(rec, evs)
		}
		return w.PutObjectWithOutbox(rec, evs)
	}
	var err error
	if versioned {
		err = s.Meta.PutObjectVersioned(rec)
	} else {
		err = s.Meta.PutObject(rec)
	}
	if err != nil {
		return 0, err
	}
	return insertOutboxBestEffort(s.Meta, evs)
}

func (s *Service) commitBucket(rec metadata.BucketRecord, evs []events.OutboxRecord) (int, error) {
	if len(evs) == 0 {
		return 0, s.Meta.PutBucket(rec)
	}
	if w, ok := s.Meta.(metadata.OutboxMutator); ok {
		return w.PutBucketWithOutbox(rec, evs)
	}
	if err := s.Meta.PutBucket(rec); err != nil {
		return 0, err
	}
	return insertOutboxBestEffort(s.Meta, evs)
}

func (s *Service) commitDeleteBucket(storageKey string, evs []events.OutboxRecord) (int, error) {
	if len(evs) == 0 {
		return 0, s.Meta.DeleteBucket(storageKey)
	}
	if w, ok := s.Meta.(metadata.OutboxMutator); ok {
		return w.DeleteBucketWithOutbox(storageKey, evs)
	}
	if err := s.Meta.DeleteBucket(storageKey); err != nil {
		return 0, err
	}
	return insertOutboxBestEffort(s.Meta, evs)
}

func (s *Service) commitDeleteVersion(bucket, key, versionID string, versioning bool, evs []events.OutboxRecord) (int, error) {
	if len(evs) == 0 {
		return 0, s.Meta.DeleteObjectVersion(bucket, key, versionID, versioning)
	}
	if w, ok := s.Meta.(metadata.OutboxMutator); ok {
		return w.DeleteObjectVersionWithOutbox(bucket, key, versionID, versioning, evs)
	}
	if err := s.Meta.DeleteObjectVersion(bucket, key, versionID, versioning); err != nil {
		return 0, err
	}
	return insertOutboxBestEffort(s.Meta, evs)
}

// insertOutboxBestEffort is the non-atomic path when a store cannot share a transaction.
// A store error is logged and swallowed so the metadata write that already committed
// is not reported as a failed object operation.
func insertOutboxBestEffort(meta metadata.MetadataStore, evs []events.OutboxRecord) (int, error) {
	st, ok := meta.(events.OutboxStore)
	if !ok {
		return 0, nil
	}
	var dropped int
	for _, ev := range evs {
		n, err := st.InsertOutbox(context.Background(), ev)
		if err != nil {
			slog.Error("event outbox insert failed after metadata commit", "event_id", ev.ID, "err", err)
			continue
		}
		dropped += n
	}
	return dropped, nil
}
