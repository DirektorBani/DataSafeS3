# DataSafeS3 vs alternatives (verified claims)

Community Edition comparison for typical **governed self-hosted S3** deployments. Claims link to regression evidence or docs.

| Capability | DataSafeS3 CE | MinIO CE | Nextcloud | SeaweedFS |
|------------|-------------|----------|-----------|-----------|
| S3 API | Yes ([feature-audit S3](../testing/regression-roadmap-i18n.md)) | Yes | Partial | Yes |
| Object Lock XML | Yes (Phase 2) | Yes | No | Limited |
| LDAP + OIDC | Yes | LDAP/OIDC | Strong | No |
| WebAuthn MFA | Yes | Limited | Apps | No |
| Share links + audit | Yes | No | Strong | No |
| Self-hosted console | Yes | Yes | Yes | Minimal |
| File sync / collaboration | Partial (web workspace + desktop CLI) | No | **Yes** | No |
| Petabyte erasure scale | No | **Yes** | No | Partial |

## Where we compete

- Backup landing zones with retention (restic, Velero) — see [backup use-case](../use-cases/en/backup-storage.md)
- Corporate file sharing with SSO — [corporate file storage](../use-cases/en/corporate-file-storage.md)
- Kubernetes in-cluster S3 — [k8s object storage](../use-cases/en/k8s-object-storage.md)

## Where we do not compete

- Hyperscale multi-PB erasure clusters (MinIO/Ceph territory)
- End-user file sync and real-time collaboration (Nextcloud territory)
- Multi-cloud CDN edge delivery

## Performance

See published benchmarks: [performance-benchmarks.md](../testing/performance-benchmarks.md)

## Evidence

- Regression gate: `scripts/feature-audit-test.ps1` (target 93/93+)
- Internal assessment: [competitive-assessment-2026.md](../analysis/competitive-assessment-2026.md)
