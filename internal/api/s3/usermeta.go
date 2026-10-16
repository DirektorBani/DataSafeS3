package s3

import "context"

type userMetaKey struct{}
type contentTypeKey struct{}

// WithUserMetadata attaches x-amz-meta-* headers to a write.
func WithUserMetadata(ctx context.Context, meta map[string]string) context.Context {
	if len(meta) == 0 {
		return ctx
	}
	return context.WithValue(ctx, userMetaKey{}, meta)
}

func userMetadataFrom(ctx context.Context) map[string]string {
	meta, _ := ctx.Value(userMetaKey{}).(map[string]string)
	return meta
}

// WithContentType attaches the object content type to a multipart upload.
func WithContentType(ctx context.Context, contentType string) context.Context {
	if contentType == "" {
		return ctx
	}
	return context.WithValue(ctx, contentTypeKey{}, contentType)
}

func contentTypeFrom(ctx context.Context) string {
	ct, _ := ctx.Value(contentTypeKey{}).(string)
	return ct
}
