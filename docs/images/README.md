# Images / Изображения

## UI screenshots (current)

Product screenshots captured from a running DataSafeS3 instance:

| File | Page |
|------|------|
| [screenshots/dashboard.png](screenshots/dashboard.png) | Dashboard |
| [screenshots/buckets.png](screenshots/buckets.png) | Buckets list |
| [screenshots/object-browser.png](screenshots/object-browser.png) | Object browser |
| [screenshots/tenants.png](screenshots/tenants.png) | Tenant management |
| [screenshots/users.png](screenshots/users.png) | User management |
| [screenshots/settings.png](screenshots/settings.png) | System settings |
| [screenshots/gateway.png](screenshots/gateway.png) | Gateway / S3 replication |
| [screenshots/activity.png](screenshots/activity.png) | Activity / audit log |
| [screenshots/monitoring.png](screenshots/monitoring.png) | Grafana monitoring |

Regenerate: `node scripts/capture-screenshots.mjs` (user guide) · `cd scripts/screenshots && npm run capture` (README / marketing set)

**Last capture:** 2026-06-19 (Playwright, local stack `localhost:8080`)

## Diagrams

Architecture diagrams use **Mermaid** embedded in Markdown. See [diagrams/README.md](../diagrams/README.md).

## Legacy

Older UI screenshots remain in [user-guide/images/](../user-guide/images/). Prefer `screenshots/` for README and marketing docs.
