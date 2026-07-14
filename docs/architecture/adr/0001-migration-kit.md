# ADR 0001: MinIO → DataSafeS3 migration kit

**Status:** Accepted  
**Date:** 2026-07-10  
**Release:** v1.1.1  

## Context

Operators leaving MinIO-compatible deployments need a documented cutover path to DataSafeS3 without claiming byte-perfect API or IAM import.

## Decision

Ship a **DIY migration kit**: ops guide (EN/RU), rclone/aws-cli cookbook, cutover checklist (`internal/migrate`), and `scripts/migrate/minio-cutover-smoke.ps1`. Object bytes move via standard S3 sync tools; DataSafe does not embed a MinIO protocol importer.

## Consequences

- Fast CE patch delivery; low support burden.
- IAM/policies remapped manually (documented).
- Future verify helpers may move from scripts into `internal/migrate` without changing the operator story.
