package s3

import "context"

type objectEventsKey struct{}

// WithObjectEvents marks an S3 SigV4 object call. put, copy, delete (including
// delete markers), and multipart complete then write ObjectCreated or
// ObjectDeleted rows when a sink is registered. Internal callers (trash,
// inventory, lifecycle) leave the flag unset and do not write those rows.
func WithObjectEvents(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, objectEventsKey{}, true)
}

// ObjectEventsRequested reports whether ctx was marked by WithObjectEvents.
func ObjectEventsRequested(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(objectEventsKey{}).(bool)
	return v
}
