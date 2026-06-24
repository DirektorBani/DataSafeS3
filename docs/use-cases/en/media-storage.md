English | **[Русский](../ru/media-storage.md)**

# Media repository

## Problem

Video, images, and audio need high-throughput upload, controlled public delivery, and large-object multipart support.

## Solution

Use DataSafeS3 as the S3 backend for transcode pipelines and CMS:

```mermaid
flowchart LR
  uploader[Transcode / CMS]
  ds[DataSafeS3]
  cdn[CDN optional]
  uploader -->|multipart S3| ds
  ds -->|public-read or presigned| cdn
```

1. Bucket `media-assets` with `public-read` or private + presigned URLs
2. S3 multipart upload for files larger than 5 MB
3. [Quotas](../../administrator-guide/en/quotas.md) per team
4. Throughput and disk monitoring in [Grafana](../../operations-guide/en/monitoring.md)
5. [Gateway replication](../../administrator-guide/en/replication.md) for disaster copies

## Result

S3-native media pipeline with governance, monitoring, and optional public delivery path.
