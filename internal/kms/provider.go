// Package kms wraps per-object data keys. E01 implements the local provider.
// Vault Transit is a later provider behind the same interface.
package kms

import (
	"context"
	"errors"
	"time"
)

// ErrUnavailable means the key store could not wrap or unwrap a DEK.
var ErrUnavailable = errors.New("KMS unavailable")

// Provider wraps and unwraps data keys. Implementations must not log key bytes.
type Provider interface {
	Name() string
	Wrap(ctx context.Context, keyID string, dek []byte) (wrapped []byte, version int, err error)
	Unwrap(ctx context.Context, keyID string, version int, wrapped []byte) (dek []byte, err error)
	Rewrap(ctx context.Context, keyID string, version int, wrapped []byte) ([]byte, int, error)
	Rotate(ctx context.Context, keyID string) (version int, err error)
	Destroy(ctx context.Context, keyID string) error
	Health(ctx context.Context) error
}

// DEKSource is a Provider that caches unwrapped DEKs. The cache key includes
// key id, key version, and the kms_keys generation so a poll can drop them.
type DEKSource interface {
	Provider
	UnwrapDEK(ctx context.Context, keyID string, version int, generation int64, wrapped []byte) ([]byte, error)
	Test(ctx context.Context) error
	Available() bool
	Probed() bool
	LastError() string
	// Mark records a health transition. The returned action is "", "kms_unavailable", or "kms_recovered".
	Mark(up bool, code string) string
	NoteGenerations(gens map[string]int64)
	Timeout() time.Duration
	PollInterval() time.Duration
}
