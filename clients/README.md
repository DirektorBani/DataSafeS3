# DataSafeS3 clients

| Client | Path | Stack | Status |
|--------|------|-------|--------|
| **Sync CLI** | [`cmd/datasafe-sync`](../cmd/datasafe-sync) | Go | **Phase 3 — shipped** |
| **Desktop** | [`desktop/`](desktop/) | Tauri 2 + sidecar | **Phase 3 — shipped** |
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

```bash
# From repo root — bundle sidecar binaries for Tauri
./scripts/build-datasafe-sync.ps1   # Windows
# ./scripts/build-datasafe-sync.sh  # Linux/macOS

cd clients/desktop
npm install
npm run tauri dev
```

See [desktop/README.md](desktop/README.md).
