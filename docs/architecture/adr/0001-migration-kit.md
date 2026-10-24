# ADR 0001: MinIO → DataSafeS3 migration kit

**Status:** Superseded in part by [ADR 0007](0007-minio-s3-copier.md)  
**Date:** 2026-07-10  
**Release:** v1.1.1  

The checklist, rclone/aws-cli cookbook, and `scripts/migrate/minio-cutover-smoke.ps1` stay in force. Object copy is `storage-cli migrate from-minio` (ADR 0007). IAM import is `storage-cli migrate from-minio iam` since v1.6.0: users, groups, service accounts, and policies this server can enforce. LDAP and OIDC are reported and not imported. MinIO secrets are not copied.

## Context

Operators leaving MinIO-compatible deployments need a documented cutover path to DataSafeS3 without claiming byte-perfect API or IAM import.

## Decision

Ship a **DIY migration kit**: ops guide (EN/RU), rclone/aws-cli cookbook, cutover checklist (`internal/migrate`), and `scripts/migrate/minio-cutover-smoke.ps1`. Object bytes moved via standard S3 sync tools. ADR 0007 adds an embedded S3 copier beside this kit.

## Consequences

- Fast CE patch delivery; low support burden.
- IAM/policies remapped manually (documented).
- Future verify helpers may move from scripts into `internal/migrate` without changing the operator story.
