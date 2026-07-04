# Contributing to DataSafeS3



Thank you for improving DataSafeS3 Community Edition.



## Build and test



```powershell

go test ./...

cd web/console && npm ci && npm run build

```



### Local stack (Windows)



```powershell

# See .cursor/rules/docker-compose-d-drive.mdc — always use local-data overlay

docker compose -p datasafe --profile postgres `

  -f docker-compose.yml `

  -f docker-compose.local-data.yml `

  -f docker-compose.local-binary.yml `

  up -d postgres storage-server caddy

scripts\feature-audit-test.ps1   # requires stack on :8080 + docker-compose.audit.yml overlay

```



### Playwright E2E (CI parity)



With compose up on `http://127.0.0.1:8080`:



```powershell

cd web/console

npx playwright install chromium

npx playwright test e2e/smoke.spec.ts e2e/buckets.spec.ts e2e/settings.spec.ts e2e/files.spec.ts e2e/share.spec.ts e2e/security-mfa.spec.ts e2e/teams.spec.ts

```



**OIDC Keycloak (nightly / optional):** not a PR gate. Set `E2E_OIDC_KEYCLOAK=1`, start Keycloak (`scripts\start-keycloak-test.cmd`), then run `e2e/security-oidc-keycloak.spec.ts`. See [docs/testing/oidc-e2e.md](docs/testing/oidc-e2e.md) for flake policy and [.github/workflows/e2e-oidc.yml](.github/workflows/e2e-oidc.yml).



## QA release gate (local)

Before tagging a release:

1. Rebuild server binary for `local-binary` overlay:  
   `$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o deploy/docker/storage-server-linux ./cmd/storage-server; Remove-Item Env:GOOS,Env:GOARCH -ErrorAction SilentlyContinue`
2. Fresh volumes: `docker compose ... down -v`, clear `D:\datasafe-data\storage` and `postgres`.
3. Stack with `docker-compose.audit.yml` (+ prometheus/grafana for monitoring rows).
4. `scripts\start-minio-test.cmd` (or let feature-audit auto-start MinIO on :9100).
5. Integration sidecars for full audit: `start-ldap-test.cmd`, `start-keycloak-test.cmd`, `start-elasticsearch-test.cmd`, `start-loki-test.cmd`.
6. `scripts\feature-audit-test.ps1` — target **0 FAIL**.
7. Playwright 7 specs on `http://127.0.0.1:8080`.
8. Vault smoke (isolated): `pwsh -File scripts/vault/smoke-vault-integration.ps1` — uses ports **9001** / **5434** (no clash with main stack).

Full QA report template: `_local/qa/` (gitignored, not committed).

### v1.1.0 spec

Implementation charter: [docs/specs/v1.1.0-trust-debt-oss-growth-tz.md](docs/specs/v1.1.0-trust-debt-oss-growth-tz.md).

## Pull request checklist



- [ ] `go test ./...` passes

- [ ] Console builds when UI changed (`npm run build`)

- [ ] Playwright regression (7 CI specs) when UI/auth changed

- [ ] Feature-audit regression unchanged or extended with new checks

- [ ] User-facing docs updated in **EN and RU** when behavior changes

- [ ] No secrets or local-only roadmap specs in commits

- [ ] PO review for features ≥5.1 on product scale (see `.cursor/skills/datasafe-product-owner/`)



## Good first issues



Look for GitHub issues labeled `good first issue`. Maintainer may open these from backlog:



- **Add Playwright `teams.spec.ts` stability** — harden selectors, API setup/teardown, or parallel-safe team names.

- **Extend feature-audit row for trash or tenant RBAC** — new `Record-Test` block in `scripts/feature-audit-test.ps1` with EN/RU roadmap note.

- **Translate api-guide example to RU Python** — mirror `docs/api-guide/en/examples/python/` under `docs/api-guide/ru/examples/python/`.

- Documentation, reference-arch scripts under `scripts/reference-arch/`, and i18n gaps are also welcome.



## Local-only documentation



Some paths are gitignored: `docs/analysis/`, `_local/`, filled handoffs, audit artifacts. Product specs live under `docs/specs/` and `docs/en/specs/` + `docs/ru/specs/`.



## Community and Enterprise



- [Community ↔ Enterprise lifecycle](docs/en/enterprise/community-enterprise-lifecycle.md) · [RU](docs/ru/enterprise/community-enterprise-lifecycle.md)

- [Feature request evaluation](docs/en/enterprise/feature-request-evaluation.md) · [RU](docs/ru/enterprise/feature-request-evaluation.md)



## Code style



Match surrounding code: minimal diffs, existing naming, no unnecessary abstractions.



## Reporting issues



Use GitHub issue templates for bugs and feature requests.

