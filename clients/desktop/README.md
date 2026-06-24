# DataSafeS3 Desktop Sync (Phase 3)

Tauri 2 app with **system tray**, folder picker, background **watch** (poll or fsnotify), bucket list, conflict panel, and bundled **`datasafe-sync`** sidecar.

## Prerequisites

- [Rust](https://rustup.rs/) + Cargo
- Node.js 20+
- Go 1.25+ (to build sidecar)

## Build sidecar (required for Tauri bundle)

From repository root:

```powershell
.\scripts\build-datasafe-sync.ps1
```

```bash
./scripts/build-datasafe-sync.sh
```

Copies target-triple binaries into `src-tauri/binaries/` for Tauri `externalBin`.

## Run desktop UI (dev)

```bash
cd clients/desktop
npm install
npm run tauri dev
```

For dev without bundling, ensure `datasafe-sync` is on `PATH` (Tauri falls back when sidecar missing in dev depending on setup — prefer building sidecar first).

## CLI-only (no Tauri)

```bash
datasafe-sync login --server http://localhost:8080 --user alice --password secret
datasafe-sync sync --folder ~/DataSafeS3 --bucket files --prefix reports/ --pull --push --delete
datasafe-sync watch --folder ~/DataSafeS3 --fsnotify --interval 15s --delete
datasafe-sync conflicts --folder ~/DataSafeS3
datasafe-sync resolve --name "doc (conflict 2026-06-23-120000).pdf"
datasafe-sync token set --token JWT   # MFA / console token
```

## Features

| Feature | CLI | Desktop UI |
|---------|-----|------------|
| JWT login | yes | yes |
| Bucket picker (owned/shared) | `buckets --json` | yes |
| Prefix sync | `--prefix` | yes |
| Delete propagation | `--delete` | checkbox |
| Conflict policies | `--conflict-policy` | dropdown |
| fsnotify watch | `watch --fsnotify` | checkbox |
| Tray icon | — | yes |
| Conflict backups | `.datasafe-conflicts/` | list panel |

Uses console REST API (`POST /api/v1/admin/login`, object CRUD).

## Production builds

```bash
npm run tauri build
```

Code signing and auto-update are not configured in-tree (ops task).
