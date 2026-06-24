package storage

import (
	"context"
	"io"
	"time"
)

// ObjectInfo describes stored object metadata returned by the backend.
type ObjectInfo struct {
	Bucket       string
	Key          string
	Size         int64
	ETag         string
	ContentType  string
	LastModified time.Time
	Metadata     map[string]string
}

// PartInfo describes an uploaded multipart part.
type PartInfo struct {
	PartNumber int
	ETag       string
	Size       int64
}

// Backend stores object bytes on disk.
type Backend interface {
	PutObject(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) (etag string, err error)
	GetObject(ctx context.Context, bucket, key string) (rc io.ReadCloser, info ObjectInfo, err error)
	DeleteObject(ctx context.Context, bucket, key string) error
	StatObject(ctx context.Context, bucket, key string) (ObjectInfo, error)

	CreateMultipartUpload(ctx context.Context, bucket, key, uploadID string) error
	UploadPart(ctx context.Context, bucket, key, uploadID string, partNumber int, r io.Reader, size int64) (etag string, err error)
	ListParts(ctx context.Context, bucket, key, uploadID string) ([]PartInfo, error)
	CompleteMultipartUpload(ctx context.Context, bucket, key, uploadID string, parts []PartInfo) (etag string, err error)
	AbortMultipartUpload(ctx context.Context, bucket, key, uploadID string) error

	CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) (etag string, err error)
}
