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

## Checklist

- [ ] Backup metadata and objects
- [ ] Review changelog / migrations
- [ ] Test on staging
- [ ] Rebuild console if UI changed: `scripts\build-console.cmd`
