# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| latest release tag | yes |
| main branch (dev) | best-effort |

## Reporting a vulnerability

Please **do not** open public GitHub issues for security vulnerabilities.

1. Email **[trachyk.i@gmail.com](mailto:trachyk.i@gmail.com)** or use [GitHub Security Advisories](https://github.com/DirektorBani/DataSafeS3/security/advisories/new) with:
   - Affected component and version
   - Steps to reproduce
   - Impact assessment (if known)
2. We aim to acknowledge within **3 business days** and provide a remediation timeline within **14 days** for confirmed issues.
3. Coordinated disclosure: we prefer a 90-day window before public details unless a fix is available sooner.

## Verifying release images (cosign)

Release tags on GHCR are signed with [Cosign](https://docs.sigstore.dev/) (keyless, OIDC). Verify before deploy:

```bash
# Install cosign: https://docs.sigstore.dev/cosign/system_install/
export COSIGN_EXPERIMENTAL=1
TAG=v1.1.0

cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com

cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

SBOM files (`sbom-storage-server.cdx.json`, `sbom-console.cdx.json`) are attached to each [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Secure development

- Release images on GHCR are accompanied by **SBOM** artifacts and **Cosign** signatures (see `.github/workflows/release.yml`).
- CI runs `go test ./...`, `govulncheck`, and feature-audit regression gates.
- Dependencies are pinned in `go.mod` / `package-lock.json`.
- External assessment checklist: [pen-test preparation](docs/operations-guide/en/pen-test-preparation.md).

## Community Edition scope

All security features (HA, Object Lock, audit, STS MVP) ship under **Apache-2.0** with **no license gates**.

### v1.0.2 advisory (2026-06-28)

Release **v1.0.2** closes SSRF, OIDC token-in-URL, and default-secrets findings for Community self-hosted deployments. Operators should upgrade server and console together, review outbound URLs (webhooks, log sinks), and rotate secrets flagged by `GET /api/v1/settings/security-status`. Migration details: [upgrade guide](docs/operations-guide/en/upgrade.md#upgrading-to-v102) and [CHANGELOG](CHANGELOG.md#102---2026-06-28).

### v1.1.0 release notes (2026-07-04)

Release **v1.1.0** removes **`STORAGE_OUTBOUND_HTTP_ALLOW`** (use `STORAGE_DEV=true` on non-production only, or HTTPS in production), implements **`STORAGE_METRICS_TOKEN`** bearer auth on `/metrics`, stores share tokens hash-only, and adds Teams/MFA audit coverage. Migration: [upgrade guide § v1.1.0](docs/operations-guide/en/upgrade.md#upgrading-to-v110), [pen-test preparation](docs/operations-guide/en/pen-test-preparation.md), and [CHANGELOG](CHANGELOG.md#110---2026-07-04).

### v1.0.3 release notes (2026-06-30)

Release **v1.0.3** adds **opt-in metadata field encryption** (Community, `STORAGE_FIELD_ENCRYPTION_*`) and documents optional **Vault Agent env injection** for bootstrap secrets. Default behaviour matches v1.0.2 until encryption is enabled. `GET /api/v1/settings/security-status` now includes a `field_encryption` block; console **Admin → Settings → Security** shows the same posture. Migration: [upgrade guide § v1.0.3](docs/operations-guide/en/upgrade.md#upgrading-to-v103) and [CHANGELOG](CHANGELOG.md#103---2026-06-30).
