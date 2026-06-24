// Package extensions documents Community Edition hook points for integrators.
//
// Webhook payloads use schema version "1" with fields: event, version, timestamp, data.
// Supported activity webhook event types (extend by subscribing in Admin → Webhooks):
//   - object.created, object.deleted, object.copied
//   - share.created, share.downloaded, share.limit_reached, share.expired
//   - extension.test (diagnostic via POST /api/v1/hooks/test)
//
// Go hook: RegisterObjectUploadValidator runs before metadata commit on REST/S3 uploads.
// Build with -tags=extensions and link a validator from your plugin package.
package extensions

import (
	"context"
	"sync"
)

// UploadValidationContext is passed to upload validators.
type UploadValidationContext struct {
	Bucket      string
	Key         string
	ContentType string
	Size        int64
}

// UploadValidator may reject an upload by returning a non-nil error.
type UploadValidator func(ctx context.Context, in UploadValidationContext) error

var (
	validatorsMu sync.RWMutex
	validators   []UploadValidator
)

// RegisterObjectUploadValidator adds a validator (last registered runs last).
func RegisterObjectUploadValidator(fn UploadValidator) {
	validatorsMu.Lock()
	defer validatorsMu.Unlock()
	validators = append(validators, fn)
}

// ValidateUpload runs registered validators; returns first error.
func ValidateUpload(ctx context.Context, in UploadValidationContext) error {
	validatorsMu.RLock()
	defer validatorsMu.RUnlock()
	for _, fn := range validators {
		if fn == nil {
			continue
		}
		if err := fn(ctx, in); err != nil {
			return err
		}
	}
	return nil
}

// EventTypes returns documented webhook event type strings.
func EventTypes() []string {
	return []string{
		"object.created",
		"object.deleted",
		"object.copied",
		"share.created",
		"share.downloaded",
		"share.limit_reached",
		"share.expired",
		"extension.test",
	}
}
