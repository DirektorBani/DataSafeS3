// Package kms wraps per-object data keys. E01 implements the local provider.
// Vault Transit is a later provider behind the same interface.
package kms

import "context"

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
