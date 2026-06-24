# Contributing to DataSafeS3

Thank you for improving DataSafeS3 Community Edition.

## Build and test

```powershell
go test ./...
cd web/console && npm ci && npm run build
scripts\feature-audit-test.ps1   # requires compose stack on :8080
```

## Pull request checklist

- [ ] `go test ./...` passes
- [ ] Console builds when UI changed (`npm run build`)
- [ ] Feature-audit regression unchanged or extended with new checks
- [ ] User-facing docs updated in **EN and RU** when behavior changes
- [ ] No secrets or local-only roadmap specs in commits


## Local-only documentation

Some paths are gitignored and kept only on your machine: `docs/analysis/`, `docs/specs/roadmap/`, `docs/context/` (legacy mirror), `docs/testing/` audit artifacts, and `.cursor/` skills. Product specs live under `docs/en/specs/` and `docs/ru/specs/` (and root `docs/specs/` where mirrored).

## Community and Enterprise

Feature and customization requests follow the public lifecycle policy and evaluation template:

- [Community ↔ Enterprise lifecycle](docs/en/enterprise/community-enterprise-lifecycle.md) · [RU](docs/ru/enterprise/community-enterprise-lifecycle.md)
- [Feature request evaluation template](docs/en/enterprise/feature-request-evaluation.md) · [RU](docs/ru/enterprise/feature-request-evaluation.md)

## Code style

Match surrounding code: minimal diffs, existing naming, no unnecessary abstractions.

## Reporting issues

Use GitHub issue templates for bugs and feature requests.
