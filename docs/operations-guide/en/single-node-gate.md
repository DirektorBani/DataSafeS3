**[Русский](../ru/single-node-gate.md)** | English

# Single-node regression gate

CI job `single-node-gate` checks that a pull request still runs as one production container: embedded Bolt, `STORAGE_DEV=false`, no Postgres, NATS, or Kafka process, and an empty notification outbox when nothing is configured to receive events.

Shipped in **v1.5.0**. The job does not publish an image and does not push a tag. The resource budget still compares with the published **v1.4.1** image. The installer retag matches the current pin, **v1.5.0**, which is not on GHCR until the release tag is pushed.

## What it guarantees

On every pull request into `main`, and on a push to `main`:

1. The job builds `storage-server` from `deploy/docker/Dockerfile` and runs **only that container**. Environment matches installer profile `core`: Bolt metadata, dev mode off, a metrics bearer token, non-default JWT and S3 secret, admin password still `admin`, and `STORAGE_POSTGRES_HOST=postgres` set the way Compose sets it. No `STORAGE_NATS_URL` and no `STORAGE_KAFKA_BROKERS`. Postgres, Prometheus, and Grafana are not started.
2. Smoke, with the AWS CLI (path-style): `/healthz`, admin login, the first-run password change and `POST /api/v1/setup/complete`, S3 create-bucket / put / get / list, Activity has at least one entry, inventory job create and list, inventory CSV download, activity CSV and JSON export. Those files are zipped the same way as `scripts/collect-evidence-pack.ps1`.
3. With no sinks, `datasafe_events_outbox_pending` is 0 and the delivered, dropped, and dead counters are absent or zero. Server logs must not show a broker dial (`nats://`, `kafka`, `amqp://`, `dial tcp`, `lookup postgres`, `no such host`, `connection refused`). `/proc/net/tcp` and `tcp6` inside the container must not show an outbound socket except loopback (the image healthcheck) and connections whose local port is 9000 (inbound smoke).
4. The same smoke runs again after `./install.sh --yes --profiles core`. That path starts `storage-server` and Caddy only. Admin calls go through Caddy on port 8080. S3 stays on port 9000.
5. Idle RSS and startup-to-healthy are compared with `ghcr.io/direktorbani/datasafe-storage-server:v1.4.1` on the same runner. The job fails when the PR startup median is more than 1 second above the baseline median, or the PR idle RSS median is more than 10% above the baseline median. The table is in the GitHub step summary. That Bolt limit is not raised.
6. After the Bolt measurement, the same image is started with `STORAGE_METADATA_BACKEND=postgres` against a Postgres 16 container. Its idle RSS median is compared with the Bolt v1.4.1 median from this job and fails above that baseline + 10%. Startup of the Postgres process is printed and is not a failure.

`install.ps1 -Yes -Profiles core` is the Windows equivalent. CI parses that script on `windows-latest`. It does not boot Docker there.

## How the budget is measured

Five runs of each image, interleaved (baseline, PR, baseline, PR, …) so a slow stretch of the runner hits both. Startup is container create until `GET /healthz` returns. After a 5 second warm-up the job reads `VmRSS` of PID 1 three times, one second apart, and keeps the median of those three. The run medians are what the limits use. Minimum and maximum are printed next to them.

A runner that is slow for most of the PR samples can still fail a build that is fine on another host. That residual is why the summary includes min and max. The limits themselves are not widened in the job.

## Run it locally (Linux)

Docker, curl, and python3. The AWS CLI is used for S3; the script installs it with `apt-get` when it is missing and `sudo` works.

```bash
python3 scripts/single-node-gate.py self-test

docker build -t datasafe-storage-server:gate -f deploy/docker/Dockerfile .
docker pull ghcr.io/direktorbani/datasafe-storage-server:v1.4.1

BASE_IMAGE=ghcr.io/direktorbani/datasafe-storage-server:v1.4.1 \
PR_IMAGE=datasafe-storage-server:gate \
bash scripts/single-node-gate.sh all
```

`all` prints the budget table, then smokes the PR image on port 9000. `measure` and `smoke` are the two halves. `GATE_RUNS` (default 5) and `GATE_WARMUP_SEC` (default 5) change the sample size.

Core profile through the installer, using the image you just built (Compose `pull_policy` is `if_not_present`, so the local tag wins and nothing is pushed):

```bash
docker build -t ghcr.io/direktorbani/datasafe-console:v1.5.0 -f deploy/docker/Dockerfile.console .
docker tag datasafe-storage-server:gate ghcr.io/direktorbani/datasafe-storage-server:v1.5.0
./install.sh --yes --profiles core --project datasafe-gate --data-root "${TMPDIR:-/tmp}/datasafe-gate"
bash scripts/single-node-gate.sh smoke-installed
docker compose -p datasafe-gate -f docker-compose.yml down -v
```

CI also unpacks `git archive HEAD` (no `web/console/dist`) and runs the same core install on `PUBLIC_HTTP_PORT=18080`. `GET /` must be 200 and contain `id="root"`.

Do not point `measure` at that retagged name. The script refuses to compare two images with the same id. Pull `v1.4.1` again first.

Windows, from the repo root (Docker Desktop running):

```powershell
.\install.ps1 -Yes -Profiles core -DataRoot "$env:USERPROFILE\datasafe-data"
```

`-Yes` skips the menu, the data-root prompt, and the confirm prompt. Profile `core` leaves Postgres, monitoring, host data volumes, the identity lab, and dev mode off. If the console edge is not healthy (`/healthz` and `GET /` with `id="root"`), `-Yes` exits with an error.

## Honesty

- Outbox emptiness is the Prometheus series, not a count of Bolt keys. The pending gauge is registered at process start, so a quiet server still reports 0.
- The TCP check does not see UDP, including DNS. A name lookup that fails before a TCP connect has to show up in the log patterns above.
- Inbound clients (the smoke, Caddy, the healthcheck) are allowed. Any other non-loopback socket fails the job, including a broker port.
- The evidence zip is built by `scripts/single-node-gate.sh` from the Admin API. Linux CI does not run `collect-evidence-pack.ps1`.
- The installer step retags the PR image as `ghcr.io/direktorbani/datasafe-storage-server:v1.5.0` **locally** so `./install.sh` (default tag `v1.5.0`) runs the PR bits. The job does not push that tag. A normal install without that retag pulls the published image, which does not exist until the release tag is pushed. The budget baseline remains the published `v1.4.1` image.
- Login to `ghcr.io` before the baseline pull is optional. Fork pull requests skip it, and a failed login does not fail the job. The `v1.4.1` image is public.
- The Windows CI job only checks that `install.ps1` parses and documents `-Yes -Profiles core`.
- Startup includes Docker create time. It is the same procedure for both images; the limit applies to the difference of medians.
