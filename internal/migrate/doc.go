// Package migrate copies buckets from a MinIO-compatible S3 endpoint and prints
// the operator checklist. storage-cli migrate from-minio is the copier.
// rclone remains the DIY path. IAM import is out of scope.
// See docs/operations-guide/en/migrate-from-minio.md and docs/architecture/adr/0007-minio-s3-copier.md.
package migrate
