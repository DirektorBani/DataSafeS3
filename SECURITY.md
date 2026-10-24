# Security Policy

Русская политика: [docs/ru/security-policy.md](docs/ru/security-policy.md).



## Supported versions

| Version | Supported |
|---------|-----------|
| latest release tag | yes |
| previous minor line (e.g. 1.6.x while 1.7.x is current) | security fixes only, for 12 months after the newer minor line is released |
| main branch (dev) | best-effort |

## Reporting a vulnerability

Please **do not** open public issues for security vulnerabilities.

1. Email **[trachyk.i@gmail.com](mailto:trachyk.i@gmail.com)** with:
   - Affected component and version
   - Steps to reproduce
   - Impact assessment (if known)
2. We acknowledge your report (first response) within **1 business day** (Moscow time) and provide a remediation timeline within **14 days** for confirmed issues.
3. Coordinated disclosure: we prefer a 90-day window before public details unless a fix is available sooner.

## Verifying release artifacts

Release images, packages and the Helm chart are built by the GitVerse workflow `.gitverse/workflows/release.yaml` and published only to the GitVerse registries (`gitverse.ru/direktorbani/datasafe-storage-server`, `gitverse.ru/direktorbani/datasafe-console`, apt/dnf/helm under `https://gitverse.ru/api/packages/DirektorBani/`). Images are **not** cryptographically signed (no cosign or other signature is published). What you can check:

- **Digest.** Each release page lists the image digests and `SHA256SUMS.txt` for the `.deb`/`.rpm` files. Compare them with the registry: `crane manifest gitverse.ru/direktorbani/datasafe-storage-server:vX.Y.Z` (the index has `linux/amd64` and `linux/arm64`). Pin deployments by digest (`image@sha256:...`) to be independent of tag changes.
- **SBOM.** `sbom-vX.Y.Z.tgz` on the GitVerse release contains CycloneDX files for the Go modules, the console npm packages and both images, with a `SHA256SUMS.txt` for the archive contents. Licence notices: [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).
- **Packages.** The apt repository is signed with a GPG key (`https://gitverse.ru/api/packages/DirektorBani/debian/repository.key`); `apt` verifies it. RPM packages are not GPG-signed; repository metadata is signed and `SHA256SUMS.txt` on the release page covers the files.
- **Transport.** All downloads use TLS to `gitverse.ru`.

Install steps: [docs/linux-packages.md](docs/linux-packages.md). How a release is built: [docs/ru/build.md](docs/ru/build.md).

## Secure development

- Every release carries **SBOM** files (CycloneDX) for the Go modules, the console and both images, built by `.gitverse/workflows/release.yaml` (see [docs/ru/build.md](docs/ru/build.md)).
- CI runs `go test ./...`, `govulncheck`, and feature-audit regression gates.
- Dependencies are pinned in `go.mod` / `package-lock.json`.
- External assessment checklist: [pen-test preparation](docs/operations-guide/en/pen-test-preparation.md).

### v1.4.1 dependency fixes (2026-10-11)

Release **v1.4.1** bumps the Go toolchain to **1.26.9** and `golang.org/x/text` to **v0.41.0**. `govulncheck` on Go 1.26.8 reported GO-2026-6629 (`golang.org/x/text@v0.38.0`) and net/http standard-library issues GO-2026-6617, GO-2026-6613, GO-2026-6612, GO-2026-6611, GO-2026-6610, GO-2026-6608 (and the rest of that Go 1.26.8 set). Those are closed by the versions above. Pins at that release: `go.mod` `go 1.26.9`, `deploy/docker/Dockerfile` (`golang:1.26.9-alpine`), and `setup-go` in CI. Details: [CHANGELOG](CHANGELOG.md#141---2026-10-11).

### v1.7.1 (2026-10-24)

Go toolchain **1.26.10** closes [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809) (`crypto/x509`, traced via `webauthn.FinishLogin`). `golang.org/x/crypto` v0.58.0 and `go-ntlmssp` v0.1.1 close the findings reported against 1.7.0; the web console moves to `zod` 4.x and `react-router-dom` 7.18.x (`npm audit --omit=dev`: 0 findings). Images use Alpine 3.24 and no longer ship the `wget` package. Remaining `npm audit` findings are in development-only tooling (vite/esbuild dev server, Tailwind 3 build chain) and are not part of the shipped bundle. Details: [CHANGELOG](CHANGELOG.md).

## Community Edition scope

All security features (HA, Object Lock, audit, STS MVP) ship under **Apache-2.0** with **no license gates**.

### v1.0.2 advisory (2026-06-28)

Release **v1.0.2** closes SSRF, OIDC token-in-URL, and default-secrets findings for Community self-hosted deployments. Operators should upgrade server and console together, review outbound URLs (webhooks, log sinks), and rotate secrets flagged by `GET /api/v1/settings/security-status`. Migration details: [upgrade guide](docs/operations-guide/en/upgrade.md#upgrading-to-v102) and [CHANGELOG](CHANGELOG.md#102---2026-06-28).

### v1.1.0 release notes (2026-07-04)

Release **v1.1.0** removes **`STORAGE_OUTBOUND_HTTP_ALLOW`** (use `STORAGE_DEV=true` on non-production only, or HTTPS in production), implements **`STORAGE_METRICS_TOKEN`** bearer auth on `/metrics`, stores share tokens hash-only, and adds Teams/MFA audit coverage. Migration: [upgrade guide § v1.1.0](docs/operations-guide/en/upgrade.md#upgrading-to-v110), [pen-test preparation](docs/operations-guide/en/pen-test-preparation.md), and [CHANGELOG](CHANGELOG.md#110---2026-07-04).

### v1.0.3 release notes (2026-06-30)

Release **v1.0.3** adds **opt-in metadata field encryption** (Community, `STORAGE_FIELD_ENCRYPTION_*`) and documents optional **Vault Agent env injection** for bootstrap secrets. Default behaviour matches v1.0.2 until encryption is enabled. `GET /api/v1/settings/security-status` now includes a `field_encryption` block; console **Admin → Settings → Security** shows the same posture. Migration: [upgrade guide § v1.0.3](docs/operations-guide/en/upgrade.md#upgrading-to-v103) and [CHANGELOG](CHANGELOG.md#103---2026-06-30).
