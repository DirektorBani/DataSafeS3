# Integration API changelog (semver in openapi.yaml info.version)

## 1.1.0 — 2026-06-22

### Added
- WebAuthn MFA endpoints (`/me/mfa/webauthn/*`, `/auth/mfa/webauthn/*`)
- Share audit activity events (`share.created`, `share.downloaded`, …)
- Federation cluster connectivity test (`POST /federation/clusters/{id}/test`)
- Extended `/healthz` fields: `read_only_mode`, `replication_lag_s`
- Locale codes `de`, `fr` on `PATCH /me/locale`

### Changed
- OpenAPI Tier A routes synchronized with P0/P1 scope

## 1.0.0

Initial Community Integration API (`docs/api/openapi.yaml`).
