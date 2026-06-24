# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| latest release tag | yes |
| main branch (dev) | best-effort |

## Reporting a vulnerability

Please **do not** open public GitHub issues for security vulnerabilities.

1. Email **security@datasafe.local** (replace with your project security contact) with:
   - Affected component and version
   - Steps to reproduce
   - Impact assessment (if known)
2. We aim to acknowledge within **3 business days** and provide a remediation timeline within **14 days** for confirmed issues.
3. Coordinated disclosure: we prefer a 90-day window before public details unless a fix is available sooner.

## Secure development

- Release images on GHCR are accompanied by **SBOM** artifacts and **Cosign** signatures (see `.github/workflows/release.yml`).
- CI runs `go test ./...`, `govulncheck`, and feature-audit regression gates.
- Dependencies are pinned in `go.mod` / `package-lock.json`.

## Community Edition scope

All security features (HA, Object Lock, audit, STS MVP) ship under **Apache-2.0** with **no license gates**.
