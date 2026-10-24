# Оверлеи Compose

English: [deploy/compose/README.md](../../deploy/compose/README.md).

Для первого дня удобнее интерактивный установщик в корне: [`install.ps1`](../../install.ps1) / [`install.sh`](../../install.sh). Описание: [установщик](../getting-started/ru/installer.md).

Основные файлы остаются в **корне репозитория**:

| Файл | Роль |
|------|------|
| [`docker-compose.yml`](../../docker-compose.yml) | Основной стек (storage-server, Caddy, Prometheus, Grafana; профиль `postgres` необязателен). `STORAGE_DEV` по умолчанию **false** |
| [`docker-compose.local-data.yml`](../../docker-compose.local-data.yml) | Привязка объектов и Postgres к `DATASAFE_DATA_ROOT` на хосте |
| [`.env.example`](../../.env.example) | Шаблон окружения, копируется в `.env` |

Остальное лежит в `deploy/compose/`, чтобы корень оставался чистым.

## Как запускать

Compose всегда запускают из **корня репозитория**. Пути томов внутри оверлеев заданы от корня (`./deploy/docker/...`): Compose этого проекта разрешает их от каталога проекта.

```bash
docker compose -p datasafe --profile postgres \
  -f docker-compose.yml \
  -f docker-compose.local-data.yml \
  -f deploy/compose/docker-compose.local-binary.yml \
  up -d
```

Шаблоны лабораторий: `deploy/compose/env/`. Копируйте в рабочий файл в корне (он в gitignore), например:

```bash
cp deploy/compose/env/.env.ha.example .env.ha
```

## Указатель оверлеев

| Оверлей | Назначение |
|---------|------------|
| `docker-compose.local-binary.yml` | Смонтировать собранный `deploy/docker/storage-server-linux` |
| `docker-compose.local-console.yml` | Взять `web/console/dist` с хоста вместо `DATASAFE_CONSOLE_IMAGE` |
| `docker-compose.dev.yml` | Консоль Vite (`--profile dev`) и явный `STORAGE_DEV=true` |
| `docker-compose.audit.yml` | Feature-audit и CI (выше предел входа, явный `STORAGE_DEV=true`) |
| `docker-compose.security-test.yml` | Более жёсткие значения для проверки безопасности |
| `docker-compose.vault.yml` | Лаборатория Vault Agent (`--profile vault`) |
| `docker-compose.vault-product.yml` | Vault и строгие секреты, близкие к production |
| `docker-compose.ha.yml` | Лаборатория HA и реплики только на чтение |
| `docker-compose.ha-local.yml` | Пути данных HA для Windows и локального диска |
| `docker-compose.ha-erasure.yml` | Лаборатория erasure |
| `docker-compose.site-repl-lab.yml` | Воркер репликации площадок (площадка A) |
| `docker-compose.site-b.yml` | Порты и данные второй площадки |

Kafka и Vault в эти оверлеи входят только когда оверлей явно добавлен. Пустой список профилей их не поднимает.

Интерфейс консоли доступен на русском и английском.
