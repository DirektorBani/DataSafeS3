# DataSafeS3 clients

| Client | Path | Stack | Status |
|--------|------|-------|--------|
| **Sync CLI** | [`cmd/datasafe-sync`](../cmd/datasafe-sync) | Go | **Phase 3 — shipped** |
| **Desktop** | [`desktop/`](desktop/) | Tauri 2 + React | **1.0 scaffold** (`desktop/1.0`) |
| **Mobile** | [`mobile/`](mobile/) | Flutter | Phase 4 backlog |
| **Mobile web** | [`mobile-web/`](mobile-web/) | Vite PWA | Phase 4 backlog |

All clients use the console REST API (JWT + `/api/v1/buckets/.../objects`).

Phase 4 scope: [docs/en/specs/file-collaboration-phase4-backlog.md](../docs/en/specs/file-collaboration-phase4-backlog.md).

## Quick start (sync CLI)

```bash
go build -o datasafe-sync ./cmd/datasafe-sync
./datasafe-sync login --server http://localhost:8080 --user USER --password PASS
./datasafe-sync sync --folder ./DataSafeS3 --bucket files --pull --push --delete
./datasafe-sync watch --folder ./DataSafeS3 --fsnotify --interval 15s
./datasafe-sync buckets --json
```

## Desktop app

Windows and Linux shell. From the repository root: `npm ci`, then `npm run tauri -w datasafe-desktop -- dev`. See [desktop/README.md](desktop/README.md). The sync CLI above is unchanged.


