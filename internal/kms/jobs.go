package kms

// Job types stored in kms_jobs. Rewrap of an object already at the target
// key version is a no-op, so a leader that resumes a cursor can repeat a batch.
const (
	JobEncryptExisting = "encrypt_existing"
	JobCryptoShred     = "crypto_shred"
)

const (
	JobPending             = "pending"
	JobRunning             = "running"
	JobCompleted           = "completed"
	JobCompletedWithErrors = "completed_with_errors"
	JobFailed              = "failed"
)

// FinishStatus is the status after the walk reaches the last object.
// A positive error count is completed_with_errors.
func FinishStatus(errors int) string {
	if errors > 0 {
		return JobCompletedWithErrors
	}
	return JobCompleted
}

const (
	ActSkip    = "skip"
	ActRewrap  = "rewrap"
	ActEncrypt = "encrypt"
	ActShred   = "shred"
)

// WorkItem is the metadata a job needs. It does not include object bytes.
type WorkItem struct {
	Key           string
	VersionID     string
	DeleteMarker  bool
	Sealed        bool
	KeyID         string
	KeyVersion    int
	Wrapped       bool
	FormatVersion int
}

// Cursor orders objects so a resumed job continues after the last saved row.
// The separator is U+001E. A NUL would be rejected by Postgres jsonb.
func Cursor(key, versionID string) string {
	return key + "\x1e" + versionID
}

// AfterCursor reports whether item sorts strictly after cursor.
func AfterCursor(cursor, key, versionID string) bool {
	if cursor == "" {
		return true
	}
	return Cursor(key, versionID) > cursor
}

// Decide is idempotent. An object already at the target key version is a skip,
// and a second shred of an object with no wrapped DEK is a skip.
func Decide(item WorkItem, jobType, keyID string, target int) string {
	if item.DeleteMarker {
		return ActSkip
	}
	if jobType == JobCryptoShred {
		if keyID != "" && item.KeyID != "" && item.KeyID != keyID {
			return ActSkip
		}
		if !item.Wrapped {
			return ActSkip
		}
		return ActShred
	}
	if item.Sealed && item.Wrapped && item.KeyID == keyID && item.KeyVersion == target {
		return ActSkip
	}
	if item.Sealed && item.Wrapped && (item.KeyID == keyID || item.KeyID == "") && item.KeyVersion != target && item.FormatVersion >= 1 {
		return ActRewrap
	}
	if !item.Sealed {
		return ActEncrypt
	}
	return ActSkip
}
