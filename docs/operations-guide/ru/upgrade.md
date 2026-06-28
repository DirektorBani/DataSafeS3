**[English](../en/upgrade.md)** | Русский

# Обновление

## Docker Compose

```bash
git pull
docker compose --profile postgres build storage-server
docker compose --profile postgres up -d
```

С local binary overlay (Windows dev):

```cmd
scripts\dev-docker-local-binary.cmd
```

## Миграции

Миграции PostgreSQL выполняются автоматически при старте `storage-server` (`internal/metadata/postgres/migrations/`).

## Откат

1. Остановить стек
2. Восстановить предыдущий binary/image и backup данных
3. Запустить стек

## Проверка образов релиза (cosign)

Перед обновлением на тег проверьте подписи GHCR (см. [SECURITY.md](../../../SECURITY.md)):

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

SBOM прикреплены к каждому [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Чеклист

- [ ] Backup метаданных и объектов
- [ ] Проверить changelog / миграции
- [ ] Тест на staging
- [ ] Пересобрать консоль при изменении UI: `scripts\build-console.cmd`
