# Changelog

All notable changes to DataSafeS3 are documented in this file.

## [Unreleased]

## [1.4.0] - 2026-08-24

### Added

- **Governance Evidence Pack (CE)** — Admin storage inventory CSV jobs (`POST/GET /api/v1/inventory/jobs`, download); Activity export CSV/JSON (`GET /api/v1/activity/export`); Object Lock / retention / blocked-delete activity codes; optional activity trail GC via `STORAGE_ACTIVITY_RETENTION_DAYS` (default 90).
- **S3 path audit** — SigV4 `DeleteObject` blocked by retention/legal hold now writes `object_delete_blocked` to Activity (same as Admin JSON delete).
- **Console** — Activity Export buttons; bucket Settings → Storage inventory CSV with optional dest-bucket (admin).
- **Helper** — `scripts/collect-evidence-pack.ps1` assembles inventory + activity + settings into a folder/zip.
- **Docs** — EN/RU [governance evidence](docs/use-cases/en/governance-evidence.md) checklist; audit guide updates.
- **Tests** — Go evidence-pack coverage (inventory, activity export, S3 delete-blocked, retention purge); Playwright smoke `e2e/evidence-pack.spec.ts`.

### Changed

- Activity retention worker runs once at process start, then hourly (was hourly-only).
- Inventory `schedule` other than `manual` returns **501** (cron not shipped).

### Honesty

- Inventory is Admin-first CSV for operator evidence — **not** AWS S3 Inventory + Athena / Parquet analytics; cron/durable queue deferred.
- Activity retention GC is disk hygiene — **not** a WORM / certified compliance audit store.
- Evidence pack does **not** claim ISO/SOC certification or multi-AZ magic.
- Read-only **auditor** role is not in this slice (admin exports the pack for the auditor).

## [1.3.0] - 2026-08-03

Minor release: **cluster installer (Waves 1–2)** and **SSH Docker lab release gate** (live Apply, Patroni promote, unicast keepalived VIP — 0 SKIP), plus cluster Grafana metrics and root install entrypoints.

### Added

- **Cluster installer (Waves 1–2)** — inventory/plan/render/apply scripts (`scripts/cluster/`), templates under `deploy/cluster/templates/`, SECURITY notes, and installer tests (`scripts/tests/cluster-installer-w1|w2`).
- **SSH Docker lab release gate** — offline-capable node image + `up-ssh.sh` / `run-apply-ssh.sh` / `run-drills-ssh.sh` for live Apply, Patroni promote ≤60s, and unicast keepalived VIP move (**0 SKIP**). See `deploy/cluster/lab/README.md`.
- **Cluster observability** — Prometheus gauges `datasafe_cluster_node_*` / overall / HA; Grafana dashboard `datasafe-cluster.json`; optional Prometheus `file_sd` targets.
- **Root install entrypoints** — `install.ps1` / `install.sh` / `install.cmd` with optional `identity` profile; getting-started installer docs (EN/RU).
- **Public login options** — `GET` login-options for LDAP/OIDC console buttons without leaking secrets.
- **Compose layout** — overlays and env examples under `deploy/compose/` (paths updated in scripts/docs).

### Changed

- HAProxy cluster templates bind VIP addresses only; Patroni listens on fabric `NODE_IP` so colocated LB + Postgres do not clash.
- Lab Apply deploys storage-server after etcd/Patroni quorum; secrets file uses Patroni **superuser** password.

### Honesty

- SSH lab unicast VRRP proves the Apply/drill path on Docker Desktop; it is **not** bare-metal L2 multicast parity.
- Offline compose lab may still SKIP VIP/Patroni by design — use the SSH lab for the no-skip gate.
- Grafana cluster panels reflect `Cluster.Nodes` configured in Admin (empty → single local fallback).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.3.0`, `ghcr.io/direktorbani/datasafe-console:v1.3.0`.

## [1.2.0] - 2026-07-21

Minor release: **immutable backup golden path** (Object Lock + versioning) and audit hygiene (AUD-12 / AUD-16). Merges the former patch hygiene slice into one operator-facing minor.

### Added

- **Immutable backup use-case** — [EN](docs/use-cases/en/immutable-backup.md) / [RU](docs/use-cases/ru/immutable-backup.md); linked from backup storage and MinIO migration guides.
- **Admin/console `retention_mode`** — `GOVERNANCE` | `COMPLIANCE` on bucket Object Lock settings (GET/PUT + console selector).
- **Versioning Suspend in console** — Admin API `versioning_suspended` + UI checkbox (AC-VER-1a).
- **Feature-audit rows** — `retention_mode` round-trip, versioning suspended flag, folder-delete `object_count` on 409.
- **ADR-0003 Accepted** — [immutable backup golden path](docs/architecture/adr/0003-immutable-backup-path.md).

### Changed

- **AUD-12** — roadmap marked done (gateway public-read console indicators already shipped).
- **Roadmap** — bucket versioning marked **done** (Enabled/Suspended in console).

### Fixed

- **AUD-16** — folder delete confirm dialog and toast surface API `object_count` from HTTP 409 (`ApiError` body).

### Deferred (not in this tag)

- Scripted HA promote hardening (A5), alpine binary smoke (AUD-21), log-sink error surfacing (AUD-22) — candidate for a follow-up patch.

> Honesty: Not WORM without Object Lock enabled. Not 100% AWS Object Lock parity. Scripted HA remain lab — not automatic multi-AZ.

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.2.0`, `ghcr.io/direktorbani/datasafe-console:v1.2.0`.

## [1.1.1] - 2026-07-14

Patch release: MinIO migration kit, monitoring doc/screenshot refresh, public specs redirect, `storage-cli migrate checklist`, and AUD-19 versioning rows in feature-audit.

### Added

- **MinIO migration kit** — ops guide ([EN](docs/operations-guide/en/migrate-from-minio.md), [RU](docs/operations-guide/ru/migrate-from-minio.md)), rclone example config, cutover checklist (`internal/migrate`), smoke script [`scripts/migrate/minio-cutover-smoke.ps1`](scripts/migrate/minio-cutover-smoke.ps1), suite [`test-minio-migration-kit.ps1`](scripts/migrate/test-minio-migration-kit.ps1).
- **`storage-cli migrate checklist`** — print MinIO cutover checklist to stdout (`storage-cli migrate checklist [minio]`).
- **Architecture ADRs** — [docs/architecture/adr/](docs/architecture/adr/) (migration kit Accepted; EventSink, immutable backup, inventory, scripted promote Proposed).
- **Extension stubs** — `internal/events.EventSink`, `internal/inventory` types, `internal/ha/promote` (no runtime behavior yet).
- **AUD-19** — feature-audit: versioning enable → put two versions → list versions → get by `versionId`.

### Changed

- **Monitoring docs / screenshots** — Grafana Overview caption and HTTP/S3 + Host panel descriptions (ops, admin, user guide EN/RU); refreshed `monitoring.png` / `grafana.png`.
- **Public `docs/specs/`** — internal TZ files removed from git; [README redirect](docs/specs/README.md) points to `D:\datasafe_tz\`.

> Not a claim of 100% MinIO API parity. IAM/policies remapped manually. Object sync uses rclone/aws-cli.

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.1.1`, `ghcr.io/direktorbani/datasafe-console:v1.1.1`.

## [1.1.0] - 2026-07-05

Trust-debt and OSS-growth release: security hardening C01–C21, Teams/MFA console improvements, feature-audit coverage, **HA v2 CE lab foundation**, and **trusted cluster** multi-site pairing (mTLS, replication rules, federation `cluster_id`).

> **Lab disclaimer:** HA v2 and trusted clusters are **CE lab foundations** (compose scripts, separate ports, pairing lab). They are **not** production multi-AZ HA, not an automatic failover orchestrator, not Patroni-certified clustering, and not a turnkey multi-region product.

### Security

- **`STORAGE_OUTBOUND_HTTP_ALLOW` removed** — outbound HTTP/private targets allowed only when `STORAGE_DEV=true` (non-production). Production integrations must use public HTTPS endpoints.
- **`STORAGE_METRICS_TOKEN`** — when set, `GET /metrics` requires `Authorization: Bearer <token>`; empty token keeps legacy open mode with startup warning in production.
- **Share link tokens** — stored as SHA-256 hash only (`token_hash`); plaintext returned once on create. Postgres migration `013_share_token_hash` backfills existing links; Bolt uses hash index with legacy plaintext fallback for pre-upgrade data.
- **Pen-test preparation** — operator checklist ([EN](docs/operations-guide/en/pen-test-preparation.md), [RU](docs/operations-guide/ru/pen-test-preparation.md)) for external assessments.
- **Automated mTLS cluster pairing** — join tokens (`dsjoin_*`, 15 min, single-use) stored as **hash only**; trust via mutual TLS and CA exchange (no manual fingerprint gate).
- **Cluster PKI** — per-deployment CA and client certs on disk (`STORAGE_CLUSTER_CERT_DIR`); private keys **never** in Postgres/Bolt.
- **Cert lifecycle** — 90-day client cert TTL; leader-only rotator renews at ~75 days; revoke updates CRL and stops workers.
- **Cluster metadata at rest** — field encryption paths for cluster/site-replication secrets (`enc:v1:`).

### Added

- **Teams (admin API + console)** — `GET/POST/PUT/DELETE /api/v1/teams`, member management; Admin → Teams UI ([EN](web/console/src/locales/en/teams.json)). OpenAPI paths in `docs/api/openapi-full.yaml`.
- **MFA setup wizard** — console profile flow for TOTP enrollment and verification (`e2e/security-mfa.spec.ts` smoke).
- **Feature audit** — extended to **111** checks; Grafana panel smoke; **AUD-15** tenant matrix; **AUD-18** trash restore; trusted-cluster pairing and federation `cluster_id` slices.
- **Playwright CI regression** — 7 specs on PR (`smoke`, `buckets`, `settings`, `files`, `share`, `security-mfa`, `teams`); OIDC Keycloak browser flow moved to nightly [e2e-oidc.yml](.github/workflows/e2e-oidc.yml) (optional `E2E_OIDC_KEYCLOAK=1`, see [docs/testing/oidc-e2e.md](docs/testing/oidc-e2e.md)).
- **API guide examples** — Go S3 SDK (`docs/api-guide/en/examples/go/`) and Python Admin JWT list-buckets script; CI compile check.
- **Reference-arch backup smoke** — `scripts/reference-arch/backup-restore.ps1`; linked from [backup-storage use-case](docs/use-cases/en/backup-storage.md) (EN/RU).
- **Getting started stubs** — German (`docs/getting-started/de/`) and French (`docs/getting-started/fr/`).
- **GHCR on `main`** — `.github/workflows/publish-main.yml` pushes `:main` and `:sha-*` image tags.
- **Contributing guide** — [CONTRIBUTING.md](CONTRIBUTING.md) with local stack, Playwright list, OIDC policy, good first issues.
- **HA v2 (CE)** — erasure object backend (`STORAGE_OBJECT_BACKEND=erasure`), Postgres leader lock (`STORAGE_HA_ENABLED`), site replication Admin API + console; lab scripts under `scripts/ha/`. (Internal HA TZ moved off public `docs/specs/` — see [docs/specs/README.md](docs/specs/README.md).)
- **Trusted clusters** — `GET/POST /api/v1/clusters/…` (pairing, revoke, rotate, replication-rules); Console **Clusters** page; Playwright [`trusted-clusters.spec.ts`](web/console/e2e/trusted-clusters.spec.ts).
- **Trusted-cluster replication** — mTLS S3 client to paired peers; `STORAGE_TRUSTED_CLUSTER_REPL_ENABLED` (default `true`); migrations `017`–`019`.
- **Federation `cluster_id`** — each federation peer scoped to local or trusted remote cluster.
- **Parallel multipart uploads** — console concurrency 4 for large files.
- **Load balancer templates** — [Caddy multi-cluster LB](deploy/caddy/multi-cluster-lb.md); Helm [`caddy-lb.yaml`](deploy/helm/datasafe/templates/caddy-lb.yaml).
- **Documentation (EN/RU)** — [trusted clusters ops](docs/operations-guide/en/trusted-clusters.md), [admin console guide](docs/administrator-guide/en/trusted-clusters.md), updated user guide §8.

### Changed

- **Helm** — `storageServer.config.metricsToken` maps to `STORAGE_METRICS_TOKEN`.
- **Prometheus example** — bearer scrape config in `deploy/docker/prometheus.yml`.
- **Security self-assessment** — metrics token and outbound policy notes ([EN](docs/operations-guide/en/security-self-assessment.md), [RU](docs/operations-guide/ru/security-self-assessment.md)).
- **Site replication worker** — rules with `trusted_cluster_id` use mTLS transport.

### Migration

See [upgrade guide § v1.1.0](docs/operations-guide/en/upgrade.md#upgrading-to-v110) (EN/RU). Postgres migrations `013`–`019` apply on start. Field encryption v2 is **not** in this release.

**Trusted clusters:** set `STORAGE_CLUSTER_ID` and `STORAGE_CLUSTER_ENDPOINT` (reachable from remote site — on Docker Desktop use `host.docker.internal`, not `127.0.0.1`). Backup `{STORAGE_DATA_DIR}/cluster-certs/`. See [trusted clusters upgrade](docs/operations-guide/en/upgrade.md#trusted-clusters-post-v110).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.1.0`, `ghcr.io/direktorbani/datasafe-console:v1.1.0`.

## [1.0.3] - 2026-06-30

Trust-and-quality release: optional metadata field encryption (CE), Vault env-injection ops pattern, CI/Postgres regression hardening, security console panel, and `STORAGE_OUTBOUND_HTTP_ALLOW` sunset timeline.

### Added

- **Field encryption (metadata at rest)** — opt-in X25519 envelope for access keys, gateway credentials, and system-config secrets (`enc:v1:`). **Community Edition**, no license gate. Ops guide ([EN](docs/operations-guide/en/field-encryption.md), [RU](docs/operations-guide/ru/field-encryption.md)), Postgres migration `012_field_encryption`, [scripts/crypto/](scripts/crypto/README.md). Vault Transit / HSM for KEK — Enterprise phase 2+.
- **HashiCorp Vault (env injection)** — optional Agent / Injector pattern; maps KV v2 to existing `STORAGE_*` env (no in-app Vault SDK). Guide ([EN](docs/operations-guide/en/secrets-vault.md), [RU](docs/operations-guide/ru/secrets-vault.md)), Compose overlays (`deploy/compose/docker-compose.vault.yml`, `deploy/compose/docker-compose.vault-product.yml`), [deploy/vault/](deploy/vault/README.md), Helm [values-vault-agent.yaml](deploy/helm/datasafe/examples/values-vault-agent.yaml).
- **Console** — Admin → Settings → **Security** posture panel (`GET /api/v1/settings/security-status`, including `field_encryption` block); gateway health shows `public_read_rules` count.

### Changed

- **CI** — `e2e-smoke` mirrors feature-audit compose (`--profile postgres`, health wait); gates `e2e/smoke.spec.ts` only.
- **CI** — Postgres 16 service for `go test` with `TEST_POSTGRES_DSN`; nullable `team_id` FK integration test.
- **SSRF** — regression matrix for `STORAGE_OUTBOUND_HTTP_ALLOW` in prod vs dev.
- **Release workflow** — GitHub Release body from CHANGELOG `[version]` section (`body_path`).

### Deprecated

- **`STORAGE_OUTBOUND_HTTP_ALLOW`** — scheduled removal in **v1.1.0**; migration timeline in upgrade guide.

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.3`, `ghcr.io/direktorbani/datasafe-console:v1.0.3`.

## [1.0.2] - 2026-06-28

Security patch: SSRF hardening, production secrets guidance, object key validation, OIDC exchange flow, rate limiting, and supply-chain updates.

### Security

- **SSRF** - Outbound URL policy (`internal/security/urlpolicy`) for log sinks, hook tests, webhooks, and bucket notifications (DS-SEC-003, DS-SEC-004, DS-SEC-027).
- **OIDC** - Callback redirects with one-time `exchange_code` instead of JWT in query string; `POST /api/v1/auth/oidc/exchange` (DS-SEC-005).
- **OIDC ROPC** - `STORAGE_OIDC_ROPC_ENABLED` gates `POST /api/v1/auth/oidc/password-login` (DS-SEC-013).
- **Object keys** - `ValidateObjectKey` rejects path traversal and control characters on S3 PUT/GET/DELETE (DS-SEC-024).
- **CORS** - Configurable allowlist via `STORAGE_CORS_ALLOWED_ORIGINS` (DS-SEC-001).
- **Rate limiting** - Login endpoints limited per IP (`STORAGE_RATE_LIMIT_LOGIN`, `STORAGE_RATE_LIMIT_WINDOW`) (DS-SEC-002).
- **MFA** - Optional dedicated `STORAGE_MFA_ENCRYPTION_KEY` with JWT secret fallback (DS-SEC-026).
- **LDAP** - `STORAGE_LDAP_REQUIRE_TLS=true` rejects `ldap://` URLs in settings (DS-SEC-025).
- **Secrets** - Helm `values-production.yaml` hardening template; `GET /api/v1/settings/security-status` lists weak env vars (DS-SEC-006).
- **Go** - Toolchain bumped to Go 1.26.4; `govulncheck` in CI (DS-SEC-007).

### Changed

- Console login handles `?exchange_code=` from OIDC callback (`?token=` deprecated).
- Operator docs (EN/RU): outbound URL policy, LDAP TLS, security hardening checklist, upgrade guide v1.0.2.

### Migration

Upgrading from v1.0.1 requires a **paired** storage-server and console update. OIDC IdP redirect URIs stay the same, but the browser no longer receives a JWT in the URL; the console redeems a one-time code via `POST /api/v1/auth/oidc/exchange`. Review outbound integration URLs (Loki, Elasticsearch, webhooks): production now requires public HTTPS unless `STORAGE_OUTBOUND_HTTP_ALLOW=true` or `STORAGE_DEV=true` for local dev. Login automation may need a higher `STORAGE_RATE_LIMIT_LOGIN` or retry logic (default 10/min per IP). Rotate `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY`, and `STORAGE_ADMIN_PASSWORD` before production; use `GET /api/v1/settings/security-status` or set `STORAGE_STRICT_SECRETS=true` to fail fast on defaults. See [upgrade guide](docs/operations-guide/en/upgrade.md#upgrading-to-v102) (EN/RU).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.2`, `ghcr.io/direktorbani/datasafe-console:v1.0.2`.

## [1.0.1] - 2026-06-28

Patch release: supply-chain hygiene, documentation sync, and minor UX fixes. No new user-facing capabilities.

### Added

- SBOM (Syft CycloneDX) for **both** release images (`storage-server` and `console`).
- GitHub Release job attaches SBOM artifacts and generates release notes on version tags.
- Operator guide: `cosign verify` steps for GHCR images (EN/RU; SECURITY.md, getting-started, operations guide).

### Changed

- Docker Compose / `.env.example` / Helm examples default to `v1.0.1` GHCR tags.
- `SECURITY.md` - real security contact (`trachyk.i@gmail.com`) and GitHub Security Advisories.
- Project status and roadmap: AUD-09 (OIDC issuer unreachable) marked **done**; AUD-08 bucket list error surfacing.
- Compose project name normalization (`datasafe`) and expanded `.gitignore` (from post-v1.0.0 maintenance).
- Swagger guide: placeholder screenshots replaced with text-only steps.

### Fixed

- **AUD-08** - Buckets page shows API errors instead of a silent empty list; bucket create returns 409 only for name conflicts (500 for server/metadata errors).

Container images (on tag): `ghcr.io/direktorbani/datasafe-storage-server:v1.0.1`, `ghcr.io/direktorbani/datasafe-console:v1.0.1`.

## [1.0.0] - 2026-06-24

First public **DataSafeS3 Community Edition** release.

### Highlights

- **S3-compatible API** - buckets, objects, multipart, versioning, presigned URLs, STS session tokens, Object Lock (WORM), storage classes, and gateway replication.
- **Web console** - object browser, admin settings, tenants and groups, MFA/WebAuthn, OIDC/LDAP SSO, setup wizard, EN/RU i18n.
- **Metadata** - Bolt (dev) and PostgreSQL (production); tenant-scoped buckets and RBAC.
- **HA & operations** - Docker Compose and Helm charts, Postgres failover automation, read-replica routing, federation sync, Prometheus/Grafana dashboards.
- **Collaboration** - shared links, file collaboration (Phases 1-3), audit and webhooks.
- **Security & supply chain** - cosign image signing, govulncheck in CI, OpenAPI 3.1 spec and drift tests.
- **Documentation** - bilingual user/admin guides, API guides, and deployment cookbooks.

Container images: `ghcr.io/direktorbani/datasafe-storage-server:v1.0.0`, `ghcr.io/direktorbani/datasafe-console:v1.0.0`.

[1.3.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.3.0
[1.2.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.2.0
[1.1.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.1.1
[1.1.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.1.0
[1.0.3]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.3
[1.0.2]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.2
[1.0.1]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.1
[1.0.0]: https://github.com/DirektorBani/DataSafeS3/releases/tag/v1.0.0
