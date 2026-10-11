package events

import "time"

// SinkStillPending is true when this sink has not been delivered or marked dead.
// An empty status is pending: rows written before a status was filled still retry.
func SinkStillPending(att SinkAttempt) bool {
	return att.Status == StatusPending || att.Status == ""
}

// CreatedBefore reports whether otherCreated/otherID is strictly earlier than
// createdAt/id. Equal timestamps use the ULID, which sorts lexicographically.
func CreatedBefore(otherCreated time.Time, otherID string, createdAt time.Time, id string) bool {
	other := otherCreated.UTC()
	created := createdAt.UTC()
	if other.Before(created) {
		return true
	}
	return other.Equal(created) && otherID < id
}
