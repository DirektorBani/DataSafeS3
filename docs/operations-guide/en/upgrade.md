English | **[Русский](../ru/upgrade.md)**

# Upgrade

## Docker Compose

```bash
git pull
docker compose --profile postgres build storage-server
docker compose --profile postgres up -d
```

With local binary overlay (Windows dev):

```cmd
scripts\dev-docker-local-binary.cmd
```

## Migrations

PostgreSQL schema migrations run automatically on `storage-server` start (`internal/metadata/postgres/migrations/`).

## Rollback

1. Stop stack
2. Restore previous binary/image and data backup
3. Start stack

## Verify release images (cosign)

Before upgrading to a tagged release, verify GHCR signatures (see [SECURITY.md](../../../SECURITY.md)):

```bash
export COSIGN_EXPERIMENTAL=1
TAG=v1.0.2
cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

SBOM files are attached to each [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Upgrading to v1.0.2

Release **v1.0.2** is a security patch. There are no new product features, but several defaults and auth flows change. Plan a short maintenance window and upgrade **storage-server and console together** — a v1.0.2 server with an older console (or the reverse) breaks OIDC login.

### Why this patch matters

Earlier releases put the OIDC session JWT in the browser redirect URL (`?token=…`). That token could leak via browser history, proxy logs, or referrer headers. v1.0.2 replaces it with a one-time `exchange_code` that the console redeems over POST. Separately, the server now validates every outbound HTTP URL it opens (log sinks, webhooks, hook tests, gateway checks) to block SSRF toward private networks. Login endpoints also get per-IP rate limits, and production deployments get clearer warnings when default secrets are still in place.

### OIDC callback (breaking for mixed versions)

The IdP redirect URI is unchanged (`/api/v1/auth/oidc/callback`). After login, the server redirects the browser to `/login?exchange_code=…&auth_source=oidc` instead of `?token=…`. The console calls `POST /api/v1/auth/oidc/exchange` with the code and stores the JWT from the response body. Upgrade both images before the next SSO login. Custom clients that parsed `?token=` must switch to the exchange endpoint (documented in [`openapi-full.yaml`](../../api/openapi-full.yaml)).

### Outbound URLs (SSRF policy)

In production (`STORAGE_DEV` unset or false), server-initiated HTTP must use **public HTTPS** endpoints. Plain `http://`, loopback, and RFC1918 targets are rejected unless you explicitly relax policy.

For local Loki on `http://localhost:3100`, either keep `STORAGE_DEV=true` or set `STORAGE_OUTBOUND_HTTP_ALLOW=true` (temporary — review before v1.1.0). The `docker-compose.audit.yml` overlay sets relaxed outbound and higher login limits for feature-audit runs; use it only in dev/CI, not in production.

### Login rate limits

Default: **10** login attempts per IP per minute (`STORAGE_RATE_LIMIT_LOGIN`, window `STORAGE_RATE_LIMIT_WINDOW`, default `1m`). CI scripts and load tests may hit 429 — raise the limit in a test overlay (see `docker-compose.audit.yml`) or add backoff in automation.

### New and changed environment variables

Review `.env.example` and rotate anything still at dev defaults:

| Variable | Default (prod) | Operator note |
|----------|----------------|---------------|
| `STORAGE_OUTBOUND_HTTP_ALLOW` | `false` | Allow non-HTTPS outbound (dev/Loki only) |
| `STORAGE_OIDC_ROPC_ENABLED` | `false` | Resource-owner password grant; enable only for test IdPs |
| `STORAGE_LDAP_REQUIRE_TLS` | `true` | Rejects `ldap://` URLs saved in LDAP settings |
| `STORAGE_MFA_ENCRYPTION_KEY` | (falls back to JWT secret) | Separate key for MFA secret encryption at rest |
| `STORAGE_CORS_ALLOWED_ORIGINS` | (empty) | Comma-separated browser origins for the console |
| `STORAGE_RATE_LIMIT_LOGIN` | `10` | Max auth attempts per IP per window |
| `STORAGE_RATE_LIMIT_WINDOW` | `1m` | Sliding window for login rate limit |
| `STORAGE_STRICT_SECRETS` | `false` | When `true`, refuse startup if default JWT/admin/S3 secrets remain |

Pre-flight check after upgrade: `GET /api/v1/settings/security-status` (admin JWT) lists any weak env vars still in use.

### Upgrade steps

```bash
git pull
export TAG=v1.0.2   # or build from source
docker compose --profile postgres pull   # if using GHCR images
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # or pull datasafe-console:v1.0.2
docker compose --profile postgres up -d
```

Verify cosign signatures with `TAG=v1.0.2` (see below), then smoke-test local login, OIDC (if used), and one outbound integration (webhook or log sink).

## Checklist

- [ ] Backup metadata and objects
- [ ] Review changelog / migrations
- [ ] Test on staging
- [ ] Rebuild console if UI changed: `scripts\build-console.cmd`
- [ ] v1.0.2: upgrade server **and** console together for OIDC
- [ ] v1.0.2: review outbound URLs and `STORAGE_RATE_LIMIT_LOGIN` for automation
