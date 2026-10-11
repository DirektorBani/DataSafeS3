package metadata

import (
	"github.com/DirektorBani/datasafe/internal/events"
)

// OutboxMutator is the metadata-store half of the durable notification outbox.
// Bolt and Postgres implement it. Object and bucket mutations that take events
// write the row in the same transaction as the metadata change.
type OutboxMutator interface {
	events.OutboxStore
	SetOutboxMaxPending(n int)
	PutObjectWithOutbox(rec ObjectRecord, evs []events.OutboxRecord) (dropped int, err error)
	PutObjectVersionedWithOutbox(rec ObjectRecord, evs []events.OutboxRecord) (dropped int, err error)
	DeleteObjectVersionWithOutbox(bucket, key, versionID string, versioningEnabled bool, evs []events.OutboxRecord) (dropped int, err error)
	PutBucketWithOutbox(rec BucketRecord, evs []events.OutboxRecord) (dropped int, err error)
	DeleteBucketWithOutbox(storageKey string, evs []events.OutboxRecord) (dropped int, err error)
	PutUserWithOutbox(rec UserRecord, evs []events.OutboxRecord) (dropped int, err error)
	PutTrashWithOutbox(rec TrashRecord, evs []events.OutboxRecord) (dropped int, err error)
	PutSharedLinkWithOutbox(rec SharedLinkRecord, evs []events.OutboxRecord) (dropped int, err error)
}

// Compile-time check is in outbox_bolt.go next to the methods.
var _ events.OutboxStore = (*Store)(nil)
