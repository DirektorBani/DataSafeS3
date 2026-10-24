# OpenAPI DataSafeS3 — интеграционный API

English: [api/README.md](../../api/README.md).

Спецификации OpenAPI **3.1**, написанные вручную для JSON REST (`/api/v1/*`).

| Артефакт | Путь |
|----------|------|
| Спецификация Community (Swagger, интеграторы) | [`openapi.yaml`](../../api/openapi.yaml) |
| Полный Admin API | [`openapi-full.yaml`](../../api/openapi-full.yaml) |
| Копия, которую отдаёт сервер | `internal/openapi/openapi.yaml` |
| JSON вживую | `GET /api/v1/openapi.json` |
| YAML вживую | `GET /api/v1/openapi.yaml` |
| Swagger UI | `/api/v1/docs` на порту консоли **8080** (через Caddy) и на порту API **9000** |
| Руководства | [swagger.md](swagger.md) · [openapi-full.md](openapi-full.md) |

Правил Spectral в этом репозитории нет.

**В OpenAPI нет** S3 XML API (SigV4 на порту 9000). Для него берите AWS SDK.

## Как пересобрать обе спецификации

После правки `tools/gen-openapi-yaml.go` (перечень `operations`):

```cmd
go run tools/gen-openapi-yaml.go
```

Команда пишет `openapi.yaml`, `internal/openapi/openapi.yaml` и `openapi-full.yaml`.

Пересоберите и перезапустите `storage-server`:

```cmd
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go build -trimpath -ldflags="-s -w" -o deploy\docker\storage-server-linux .\cmd\storage-server
docker compose up -d storage-server --no-deps
```

## Проверка расхождения

Проверяет, что в спецификации Community нет путей администратора, что её пути — подмножество `server.go` и что встроенная копия совпадает с `docs`:

```cmd
go test ./internal/api/... -run OpenAPI -count=1
powershell -File scripts\openapi-drift-check.ps1
```

`scripts\lint-openapi.ps1` гоняет проверку стиля спецификации, если инструмент установлен локально.

## Аутентификация

| Клиент | Как |
|--------|-----|
| Интеграции и Try it out в Swagger | `Authorization: Bearer ds_...` (токен API из консоли) |
| Человек при первом запуске | Вход в консоль → Доступ → токены API → создать |

JWT входа администратора в OpenAPI не описан: человеку — консоль, автоматизации — токен.

## Граница Community

В спецификацию Community входят health, me, бакеты и объекты, составная загрузка, ключи, предподпись. Не входят `/admin/*`, `/users`, `/settings/system` и вебхуки: они есть в полном файле `openapi-full.yaml`.

Интерфейс консоли, из которой создают токен, доступен на русском и английском.
