# AGENTS.md

## Cursor Cloud specific instructions

### What this is
DataSafeS3 is a self-hosted, S3-compatible object storage platform: a Go backend
(`cmd/storage-server`, S3 + Admin API) plus a React/Vite web console (`web/console`).
Other deliverables (`cmd/datasafe-sync`, `cmd/storage-cli`, `clients/desktop` Tauri,
`clients/mobile` Flutter, `clients/mobile-web`) are optional clients that talk to the same API.

### Recommended dev setup (native, no Docker)
Docker is NOT installed in this environment, and the `scripts/*.cmd` launchers are Windows-only.
Run the two core services natively instead — this is the cleanest dev path:

1. Storage server on `:9000` using the embedded **bolt** metadata backend (the default; no
   Postgres needed). Required env vars are listed in `.env.example` / `scripts/dev-local-server.cmd`.
   Run it from the repo root:
   ```
   mkdir -p data
   STORAGE_ADDR=:9000 STORAGE_DATA_DIR=./data STORAGE_REGION=us-east-1 \
   STORAGE_ACCESS_KEY=datasafe STORAGE_SECRET_KEY=datasafesecret \
   STORAGE_ADMIN_USER=admin STORAGE_ADMIN_PASSWORD=admin \
   STORAGE_JWT_SECRET=datasafe-jwt-secret STORAGE_DEV=true \
   go run ./cmd/storage-server
   ```
2. Web console (Vite dev, HMR) on `:5173`:
   ```
   cd web/console && npm run dev -- --host 0.0.0.0 --port 5173
   ```
   Vite proxies `/api`, `/healthz`, `/metrics` to `127.0.0.1:9000` (see `web/console/vite.config.ts`),
   so the storage server MUST be listening on `:9000` for the console to work.

### First-run gotcha (important)
On a fresh data dir the API gates most endpoints behind `setup_required` until first-run is done.
After logging in at the console (`admin` / `admin`) you MUST (1) change the admin password, then
(2) finish the setup wizard (external S3 is optional — click "Skip"). Only then do bucket/object
endpoints work. The bolt state lives in `./data` (gitignored); delete it to reset to fresh-install.

### Lint / test / build (commands already in `Makefile`)
- Test: `go test ./...` (`make test`).
- Lint: `make lint` runs `gofmt -l .` + `go vet ./...`. Note: `go vet` is clean, but `gofmt -l .`
  currently reports many pre-existing unformatted files in the committed tree, so `make lint`
  fails on the gofmt step out of the box — this is the repo's existing state, not your change.
- Console build: `cd web/console && npm run build` (`tsc -b && vite build`); E2E tests use Playwright
  (`web/console/e2e`, `playwright.config.ts`).

### Postgres backend (optional)
For a production-like run set `STORAGE_METADATA_BACKEND=postgres` and the `STORAGE_POSTGRES_*`
vars; this needs a Postgres 16 instance (normally via Docker Compose `--profile postgres`).
Not required for normal dev — bolt is the default.
