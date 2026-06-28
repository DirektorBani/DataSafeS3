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
TAG=v1.0.1
cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

SBOM files are attached to each [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Checklist

- [ ] Backup metadata and objects
- [ ] Review changelog / migrations
- [ ] Test on staging
- [ ] Rebuild console if UI changed: `scripts\build-console.cmd`
