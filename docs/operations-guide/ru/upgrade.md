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
TAG=v1.0.3
cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

SBOM прикреплены к каждому [GitHub Release](https://github.com/DirektorBani/DataSafeS3/releases).

## Обновление до v1.0.2

Релиз **v1.0.2** — security patch. Новых продуктовых функций нет, но меняются дефолты и поток аутентификации. Заложите короткое окно обслуживания и обновляйте **storage-server и console вместе** — сервер v1.0.2 со старой консолью (или наоборот) ломает OIDC-вход.

### Зачем этот патч

В прежних версиях JWT сессии OIDC попадал в URL редиректа (`?token=…`). Токен мог утечь через историю браузера, логи прокси или Referer. В v1.0.2 вместо него одноразовый `exchange_code`, который консоль обменивает через POST. Отдельно сервер проверяет каждый исходящий HTTP (log sinks, webhooks, тесты hook, gateway) — защита от SSRF во внутренние сети. На login-endpoint добавлен rate limit по IP; в production явнее предупреждаем о дефолтных секретах.

### OIDC callback (ломает смешанные версии)

Redirect URI у IdP не меняется (`/api/v1/auth/oidc/callback`). После входа браузер получает `/login?exchange_code=…&auth_source=oidc` вместо `?token=…`. Консоль вызывает `POST /api/v1/auth/oidc/exchange` и сохраняет JWT из тела ответа. Обновите оба образа до следующего SSO-логина. Собственные клиенты, парсившие `?token=`, переходят на exchange (см. [`openapi-full.yaml`](../../api/openapi-full.yaml)).

### Исходящие URL (политика SSRF)

В production (`STORAGE_DEV` не задан или false) server-initiated HTTP — только **публичный HTTPS**. Plain `http://`, loopback и RFC1918 отклоняются, пока явно не ослабите политику.

Для локального Loki на `http://localhost:3100` оставьте `STORAGE_DEV=true` или задайте `STORAGE_OUTBOUND_HTTP_ALLOW=true` (временный escape hatch — см. [планируемый sunset](#planned-deprecation-storage_outbound_http_allow) ниже). Overlay `deploy/compose/docker-compose.audit.yml` ослабляет outbound и поднимает лимит login для feature-audit — только dev/CI, не production.

### Planned deprecation: `STORAGE_OUTBOUND_HTTP_ALLOW`

| Релиз | Изменение |
|-------|-----------|
| **v1.0.2** | Строгая политика исходящих URL (`internal/security/urlpolicy`); escape hatch через `STORAGE_OUTBOUND_HTTP_ALLOW=true` |
| **v1.0.3** | Зафиксирован timeline sunset (этот раздел); в production по умолчанию `false` |
| **v1.1.0** | **`STORAGE_OUTBOUND_HTTP_ALLOW` удаляется** — `STORAGE_DEV=true` только на non-production стеках, либо публичные **HTTPS** endpoint'ы |

**До v1.1.0:** проверьте compose, Helm и `.env` на `STORAGE_OUTBOUND_HTTP_ALLOW=true`. Где возможно — переведите интеграции на HTTPS; для локального Loki/Elasticsearch используйте `STORAGE_DEV=true` только в dev/CI overlay (`deploy/compose/docker-compose.audit.yml`, не production).

**Чеклист production:** не задавайте `STORAGE_OUTBOUND_HTTP_ALLOW` (или оставьте `false`); sinks и webhooks — `https://` на публичные хосты; после обновления — `GET /api/v1/settings/security-status`.

### Rate limit на login

По умолчанию **10** попыток login с IP за минуту (`STORAGE_RATE_LIMIT_LOGIN`, окно `STORAGE_RATE_LIMIT_WINDOW`, по умолчанию `1m`). CI и нагрузочные скрипты могут получить 429 — поднимите лимит в test overlay (`deploy/compose/docker-compose.audit.yml`) или добавьте backoff в автоматизацию.

### Новые и изменённые переменные окружения

Сверьте `.env.example` и смените всё, что осталось на dev-дефолтах:

| Переменная | Default (prod) | Заметка оператору |
|------------|----------------|-------------------|
| `STORAGE_OUTBOUND_HTTP_ALLOW` | `false` | Разрешить non-HTTPS outbound (dev/Loki) |
| `STORAGE_OIDC_ROPC_ENABLED` | `false` | Resource-owner password grant; только для test IdP |
| `STORAGE_LDAP_REQUIRE_TLS` | `true` | Отклоняет `ldap://` в настройках LDAP |
| `STORAGE_MFA_ENCRYPTION_KEY` | (fallback на JWT secret) | Отдельный ключ шифрования MFA |
| `STORAGE_CORS_ALLOWED_ORIGINS` | (пусто) | Origins браузера для консоли, через запятую |
| `STORAGE_RATE_LIMIT_LOGIN` | `10` | Попыток auth с IP за окно |
| `STORAGE_RATE_LIMIT_WINDOW` | `1m` | Окно sliding window для login |
| `STORAGE_STRICT_SECRETS` | `false` | При `true` — отказ старта при дефолтных секретах |

После обновления: `GET /api/v1/settings/security-status` (admin JWT) покажет слабые env vars.

### Шаги обновления

```bash
git pull
export TAG=v1.0.3   # или сборка из исходников
docker compose --profile postgres pull   # при образах GHCR
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # или pull datasafe-console:v1.0.3
docker compose --profile postgres up -d
```

Проверьте cosign с `TAG=v1.0.3` (ниже), затем smoke-test: локальный login, OIDC (если есть), один outbound (webhook или log sink).

## Обновление до v1.0.3

Релиз **v1.0.3** — trust-and-quality patch (CI smoke, Postgres FK regression, SSRF-тесты, опциональный Vault ops pattern). **Field encryption** — новая **opt-in** возможность; по умолчанию поведение как v1.0.2.

### Field encryption (опционально)

| Тема | Действие |
|------|----------|
| По умолчанию | `STORAGE_FIELD_ENCRYPTION_ENABLED=false` — при апгрейде ничего не менять |
| Postgres | Миграция `012_field_encryption` автоматически (таблица `encryption_key_registry`) |
| Включение | KEK → env → restart; см. [field-encryption.md](field-encryption.md) |
| Проверка | `GET /api/v1/settings/security-status` → блок `field_encryption`; Admin → Settings → Security |
| Vault | Agent может инжектить `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` как bootstrap-секреты ([secrets-vault.md](secrets-vault.md)) |

**Community Edition:** KEK из env/файла, без license gate. Vault Transit / HSM — Enterprise phase 2.

Скрипты: [scripts/crypto/README.md](../../../scripts/crypto/README.md).

### Переменные v1.0.3 (field encryption)

| Переменная | Default | Заметка |
|------------|---------|---------|
| `STORAGE_FIELD_ENCRYPTION_ENABLED` | `false` | `true` — шифрование выбранных секретов метаданных |
| `STORAGE_FIELD_ENCRYPTION_ACTIVE_KEK_ID` | (не задан) | Обязателен при enabled; совпадение с active в registry |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEY` | (не задан) | Base64 raw X25519 private seed |
| `STORAGE_FIELD_ENCRYPTION_KEK_PRIVATE_KEYS` | (не задан) | JSON map для ротации |

### Шаги обновления (v1.0.3)

```bash
git pull
export TAG=v1.0.3
docker compose --profile postgres pull   # при образах GHCR
docker compose --profile postgres build storage-server
scripts/build-console.cmd                # или pull datasafe-console:v1.0.3
docker compose --profile postgres up -d
```

Проверьте cosign с `TAG=v1.0.3`, затем `GET /api/v1/settings/security-status` и Admin → Settings → Security. Field encryption — только по чеклисту [field-encryption.md](field-encryption.md).

## Обновление до v1.1.0

Релиз **v1.1.0** — minor «trust-debt»: удаление escape hatch `STORAGE_OUTBOUND_HTTP_ALLOW` и опциональная аутентификация `/metrics` через `STORAGE_METRICS_TOKEN`. Обновляйте **storage-server и console вместе**.

### Исходящие URL (breaking)

| До (v1.0.x) | После (v1.1.0) |
|-------------|----------------|
| `STORAGE_OUTBOUND_HTTP_ALLOW=true` | **Переменная удалена** |
| Loki `http://127.0.0.1` в production с allow | **HTTPS** или `STORAGE_DEV=true` только на dev/CI |

Overlay `deploy/compose/docker-compose.audit.yml` задаёт `STORAGE_DEV=true` для feature-audit — **не** для production.

**Чеклист:** убрать `STORAGE_OUTBOUND_HTTP_ALLOW` из compose/Helm/.env; перевести sinks/webhooks на `https://` где нужно.

### Аутентификация metrics

| `STORAGE_METRICS_TOKEN` | Поведение |
|-------------------------|-----------|
| Пусто | `/metrics` открыт (legacy); warning при `STORAGE_DEV=false` |
| Задан | `Authorization: Bearer <token>` обязателен |

Пример Prometheus — см. [upgrade EN](../en/upgrade.md#upgrading-to-v110) и `deploy/docker/prometheus.yml`. Helm: `storageServer.config.metricsToken`.

Консоль без bearer показывает нули на дашборде — используйте Grafana/Prometheus.

### Токены share-ссылок (метаданные)

| Backend | После обновления |
|---------|------------------|
| **PostgreSQL** | Миграция `013_share_token_hash` заполняет `token_hash`; plaintext `token` удаляется после backfill |
| **Bolt** | Новые ссылки только как hash; **fallback по legacy plaintext index** для ссылок до обновления |

Для Postgres действий не требуется. На Bolt dev старые ссылки продолжают работать; новые создаются только с hash.

### Шаги (v1.1.0)

```bash
git pull
export TAG=v1.1.0
docker compose --profile postgres pull
docker compose --profile postgres build storage-server
scripts/build-console.cmd
docker compose --profile postgres up -d
```

**Не в v1.1.0:** field encryption v2, mobile GA, Vault Transit in-process.

## Trusted clusters (после v1.1.0)

Входит в **v1.1.0**: mTLS pairing, trusted remote, правила репликации.

Миграции `017`–`019` применяются при старте `storage-server`. Переменные: `STORAGE_CLUSTER_ID`, `STORAGE_CLUSTER_ENDPOINT`, `STORAGE_TRUSTED_CLUSTER_REPL_ENABLED`. На Docker используйте `host.docker.internal`, не `127.0.0.1`, для cross-container pairing.

Подробно: [trusted-clusters.md](trusted-clusters.md) · [EN](../en/trusted-clusters.md)

## Обновление до v1.4.1

Патч поверх **v1.4.0**. Миграции БД нет. Берите storage-server и console одного тега.

### Dev mode — явный opt-in

| До (дефолт compose v1.4.0) | После (v1.4.1) |
|----------------------------|----------------|
| `STORAGE_DEV` по умолчанию `true` | В `docker-compose.yml` и `.env.example` по умолчанию `false` |
| Установщик не выключал dev mode | Пишет `STORAGE_DEV=false`, если нет `-Dev`, `--dev` или профиля `dev` |
| `/metrics` на установке по умолчанию был открыт | Установщик генерирует `STORAGE_METRICS_TOKEN`; Prometheus скрейпит с этим bearer |

Если в `.env` уже задано `STORAGE_DEV=true`, значение сохранится, пока вы его не смените. Стек, который брал старый дефолт compose (переменная не задана), выйдет из dev mode при следующем `up`.

Opt-in для локальных HTTP sink: `-Dev` / `--dev` / профиль `dev`, `STORAGE_DEV=true`, либо overlay `deploy/compose/docker-compose.dev.yml` и `docker-compose.audit.yml` (оба явно ставят `STORAGE_DEV=true`).

Профиль **identity** включает `STORAGE_DEV=true`, потому что issuer Keycloak в лаборатории — обычный HTTP. Не используйте этот профиль на production-хосте.

Мастер первого запуска не изменился: вход `admin` / `admin`, смена пароля, setup wizard (внешний S3 необязателен). Установщик не ротирует `STORAGE_ADMIN_PASSWORD`. `STORAGE_JWT_SECRET` и `STORAGE_SECRET_KEY` заменяются только пока они равны опубликованным дефолтам. `STORAGE_METRICS_TOKEN` генерируется, если его нет. Повторный запуск сохраняет уже заданные недефолтные секреты.

Ручной `docker compose` без установщика: задайте `STORAGE_METRICS_TOKEN` сами. Пока он пуст, `/metrics` остаётся открытым (legacy) и сервер пишет warning при `STORAGE_DEV=false`. Если переменная задана, сервис Prometheus читает то же значение.

Grafana на стеке установщика продолжает работать, потому что Prometheus передаёт bearer. Консоль bearer не шлёт — смотрите gauges в Grafana.

### Сборка из исходников на Windows

`.gitattributes` фиксирует LF для `*.sh` и остальных файлов, которые копируются или монтируются в Linux-контейнеры. После `git pull` пересоберите образ storage-server. Образ вырезает CR из `docker-entrypoint.sh` и стартует через `/usr/local/bin/datasafe-entrypoint`, поэтому рабочая копия с CRLF всё равно поднимается.

Уже существующий checkout на Windows не переписывает файлы, которых нет в этом pull. Один раз обновите shell-скрипты, чтобы bind-mount init Postgres (`deploy/docker/postgres/init/02-replication-hba.sh`) был с LF до первой инициализации БД:

```bash
git checkout -- "*.sh"
```

### Шаги (v1.4.1)

```bash
git pull
export TAG=v1.4.1
docker compose --profile postgres pull
docker compose --profile postgres up -d
```

Либо снова запустите `install.ps1` / `install.sh`, чтобы получить сгенерированные секреты и токен метрик. Проверьте: `GET /metrics` без bearer даёт **401**, когда токен задан, а Grafana Overview по-прежнему показывает данные.

## Обновление до v1.5.0

Минорный релиз поверх **v1.4.1**. Берите storage-server и console одного тега. `GET /healthz` и `GET /api/v1/health` отдают `version` и `commit`.

Образы `ghcr.io/direktorbani/datasafe-storage-server:v1.5.0` и `datasafe-console:v1.5.0` появляются, когда `release.yml` выполняется на git-теге. Этот коммит тег не пушит.

### Что ломается и что сделать оператору

| Тема | Что меняется |
|------|----------------|
| Postgres | Миграция `020` создаёт `event_outbox`. Миграция `021` добавляет колонки расписания gateway. Обе применяются при старте. Откат на бинарь v1.4.x оставляет их и игнорирует. Down-миграции удаляют таблицу (и события в `020`) или колонки расписания. |
| Bolt | Бакеты `event_outbox` и `event_outbox_due` создаются при следующем открытии базы. v1.4.x неизвестные бакеты не читает. |
| Sink | Заглушки Kafka и RabbitMQ удалены. Kafka доставляет только при заданном `STORAGE_KAFKA_BROKERS`. Пустое значение не делает dial. URL webhook и NATS не меняются. |
| Порты лаборатории | Erasure по умолчанию: консоль **8084**, S3 **9004**, Postgres **5444**. Site A: консоль **8086**, S3 **9006**, Postgres **5446**. Удалите старый `.env.erasure` или `.env.site-a` и скопируйте пример заново. Скопированный файл хранит прежние порты. |
| Перехват | Follower берёт аренду лидера только после её истечения. Время примерно равно оставшейся аренде (около 9 с при аренде 10 с, около 26 с при 30 с по умолчанию). Процесс, который уже держал аренду и потерял её, снова её не берёт. |

Webhook, NATS и Kafka — внешняя лента событий, а не связь между узлами storage. См. [события](../../ru/integrations/events.md) и [высокую доступность](high-availability.md).

### Что получает хост v1.4.1

Подтянуть v1.5.0 на хост v1.4.1 можно без поломки данных:

- Postgres применяет миграцию `020` (`event_outbox`) при старте. Откат на бинарь v1.4.x оставляет таблицу и игнорирует её. Down-миграция удаляет таблицу вместе с событиями.
- Bolt создаёт бакеты `event_outbox` и `event_outbox_due` при следующем открытии базы. v1.4.x неизвестные бакеты не читает.
- `STORAGE_EVENTS_RETENTION` (`24h`), `STORAGE_EVENTS_DEAD_RETENTION` (`168h`), `STORAGE_EVENTS_MAX_PENDING` (`100000`) и `STORAGE_EVENTS_MAX_ATTEMPTS` (`20`) необязательны. Пустое значение оставляет дефолт. `MAX_PENDING=0` отключает лимит.
- Настройки webhook и `STORAGE_NATS_URL` / `STORAGE_NATS_SUBJECT` не меняются. Пустой URL NATS по-прежнему не соединяется. Один узел на bolt с дефолтным конфигом соединение с брокером не открывает.
- События Admin API и объектов S3 SigV4 (put, copy, delete включая delete marker, завершение multipart) доставляются из outbox (at-least-once), когда настроен sink. Без `STORAGE_NATS_URL`, без включённого webhook и без совпавшего уведомления бакета эти записи не вставляют строку outbox и позже не воспроизводятся.
- Webhook уведомлений бакета идут через тот же диспетчер. Тело POST по-прежнему `{"event","bucket","key","size","time"}`. Необязательные фильтры prefix/suffix применяются при вставке. `time` — время коммита записи объекта. По-прежнему хранится один URL (последний topic, затем последняя queue).
- Повтор webhook: ошибки транспорта, таймауты, 5xx, 408 и 429 повторяются (`Retry-After` учитывается, потолок 5 минут). Любой другой 4xx сразу помечает этот sink dead; `last_error` содержит статус. У каждого webhook консоли по-прежнему одна строка истории на событие.
- NATS публикует с ack JetStream, если на сервере включён JetStream и stream покрывает `STORAGE_NATS_SUBJECT.<событие>`. Иначе это core publish: подтвердить получение подписчиком нельзя. Отдельной переменной нет; выбор определяется при соединении и кэшируется до переподключения.

Подробно: [event-outbox.md](event-outbox.md).

### Kafka (только по включению)

Новой миграции метаданных, кроме `020` и `021`, нет.

- Пустой `STORAGE_KAFKA_BROKERS` сохраняет прежнее поведение: нет клиента Kafka и нет соединения. Профиль установщика `kafka` выключен, пока его не передали. Профили `--yes` по умолчанию его не содержат.
- Непустой список брокеров регистрирует продюсер franz-go (`acks=all`, идемпотентный, топик `STORAGE_KAFKA_TOPIC`, по умолчанию `datasafe.events`). SASL `PLAIN` без TLS отказывается стартовать. Принимаются SCRAM-SHA-256 и SCRAM-SHA-512. Имя и пароль можно задать через `STORAGE_KAFKA_SASL_USERNAME_FILE` и `STORAGE_KAFKA_SASL_PASSWORD_FILE`.
- Тела webhook и NATS не меняются. Сообщения Kafka — отдельный JSON (`Records[]` и блок `datasafe`).
- Диспетчер держит порядок по объекту для каждого sink. Застрявший ключ блокирует только этот ключ. Если более ранняя строка стала `dead`, более поздние для этого ключа доставляются, а мёртвая — пропуск.
- Смена списка брокеров между стартами пишется в Activity как `kafka_brokers_changed` (новые адреса, не секреты SASL).
- Консоль: **Настройки → Доставка событий**. `GET /api/v1/events/delivery` и `POST /api/v1/events/test` только для администратора.

Подробно: [kafka.md](../../ru/integrations/kafka.md).

### Шаги (v1.5.0)

После публикации git-тега `v1.5.0`:

```bash
git pull
export TAG=v1.5.0
# .env (или оболочка) должны указывать опубликованные образы тега:
# DATASAFE_SERVER_IMAGE=ghcr.io/direktorbani/datasafe-storage-server:v1.5.0
# DATASAFE_CONSOLE_IMAGE=ghcr.io/direktorbani/datasafe-console:v1.5.0
docker compose --profile postgres pull
docker compose --profile postgres up -d
```

Сделайте `pull` образов server и console с тегом `v1.5.0`. Для этого обновления не запускайте `docker compose build`. Проверьте, что `GET /healthz` возвращает `"version":"1.5.0"` и `commit`, а `GET /` на порту консоли отдаёт SPA (`id="root"`). На лаборатории из трёх узлов удалите `.env.erasure` и `.env.site-a` перед пересозданием стендов.

**Известная ошибка, исправлена в 1.6.0.** В 1.5.x переменная `STORAGE_SSE_MASTER_KEY` ломает запись объектов. Бэкенд сравнивает plaintext `Content-Length` с более длинным ciphertext и отвечает ошибкой размера, поэтому SSE-объект не сохраняется. На 1.5.x переменную не задавайте. 1.6.0 пишет конверт формата 1 и отдаёт размер открытого текста. Объекты, созданные в 1.5.x без ключа, остаются открытым текстом, когда 1.6.0 стартует с ключом. SSE-блок 1.5.x без метаданных шифрования открывается этим ключом и не отдаётся как ciphertext, даже если `STORAGE_SSE_LEGACY_FORMAT0` не задан. Переменная по-прежнему включает чтение формата 0 для каждого объекта без метки.

## Обновление до v1.6.0

Сначала обновите каждый узел хранилища, и только потом включайте шифрование бакета. Процесс 1.5.0 не читает объект формата 1 (`DSSE`). Обновлять лидера последним недостаточно: лидер 1.5.0 выполняет код 1.5.0 и сам принимает `PutBucketEncryption`. В этом процессе нет проверки 409.

## Смешанный кластер

HTTP 409 `cluster_upgrade` (`cluster upgrade in progress`) отвечает узел 1.6.0, пока в кластере жив любой узел 1.5.0. Проверка не требует `system_config` → `cluster.nodes`. При поочерёдном обновлении старые узлы туда записывать не нужно.

| Что видно | 409 |
|---|---|
| Heartbeat в `cluster_node_heartbeats` с версией ниже 1.6.0 или с непустой версией, которая не разбирается как 1.6.0 и новее | Да. Неизвестная версия считается старше 1.6.0. Пустая версия от записи адреса — это не версия. |
| Неистёкшая аренда лидера `ha_leader_lock` (`lock_id=storage-server`, `holder_id`, `acquired_at`, `expires_at`), у которой `holder_id` без heartbeat 1.6.0 | Да. Это лидер 1.5.0. |
| Свежая строка в `ha_leader_attempts`, у которой `holder_id` без heartbeat 1.6.0 | Да. Это живой последователь 1.5.0. |
| Узел реестра, чей `GET /healthz` отвечает `"status":"ok"` и `"version":"1.5.0"`, или здоровый узел реестра с пустой версией | Да, если список заполнен. Недоступная запись реестра не блокирует. |

1.5.0 не пишет `cluster_node_heartbeats`. Последователь повторяет тот же `INSERT` аренды, что и лидер (`TryAcquireLeader`). Пока аренда занята, условие `WHERE` не совпадает, и `ha_leader_lock` остаётся за лидером. Миграция `030` добавляет `ha_leader_attempts` и триггер `BEFORE INSERT` на `ha_leader_lock`. Триггер срабатывает на запросе последователя и сохраняет `holder_id`, даже если обновление строки пропущено. Узел 1.6.0, который тоже повторяет аренду, попадает в эту таблицу, но его heartbeat не ниже 1.6.0, поэтому он не блокирует. Остановленный узел выпадает из двухминутного окна.

Пока проверка включена, PUT без заголовка шифрования остаётся открытым текстом. SSE-S3 в этом окне — формат 0: его читает узел 1.5.0 с тем же ключом сервера. SSE-KMS — HTTP 409.

На Postgres или HA консоль просит отметить **Все узлы обновлены до 1.6.0**, прежде чем отправить admin `PUT`. S3 `PutBucketEncryption` такой отметки не спрашивает. У него те же правила 409, что в таблице. Не включайте шифрование, пока каждый узел не станет 1.6.0. Лидер 1.5.0 выполняет код 1.5.0 и сам принимает `PutBucketEncryption`, если запрос пришёл на этот процесс.

`GET /healthz` процесса 1.6.0 сообщает не ниже `1.6.0`, даже если штамп образа ещё говорит 1.5.0. Остановленный лидер 1.5.0 отпускает аренду примерно за 30 секунд.

Если `STORAGE_SSE_MASTER_KEY` уже был записан, а следующий запуск очищает переменную, процесс отказывается стартовать. Текст ошибки говорит, что ключ отсутствует, и что нужно задать `STORAGE_SSE_MASTER_KEY` равным исходному ключу. Другое значение по-прежнему даёт `SSE master key fingerprint mismatch`.

Vault Transit выключен, пока одновременно не заданы `STORAGE_KMS_PROVIDER=vault-transit` и адрес — в окружении или в Настройки → Vault (KMS). Пока оба пусты, процесс соединение с Vault не открывает. SSE-S3 остаётся на ключе сервера и в Vault не ходит. См. [kms-vault.md](kms-vault.md).

## Чеклист

- [ ] Backup метаданных и объектов
- [ ] Проверить changelog / миграции
- [ ] Тест на staging
- [ ] Пересобрать консоль при изменении UI: `scripts\build-console.cmd`
- [ ] v1.0.2: обновить server **и** console вместе для OIDC
- [ ] v1.0.2: проверить outbound URL и `STORAGE_RATE_LIMIT_LOGIN` для автоматизации
- [ ] v1.0.3 (опционально): включить [field encryption](field-encryption.md) — KEK, env, security-status
- [ ] v1.1.0: убрать `STORAGE_OUTBOUND_HTTP_ALLOW`; настроить `STORAGE_METRICS_TOKEN` и Prometheus bearer
- [ ] v1.4.1: `STORAGE_DEV` выключен, если вы не включали его явно; в установщике или `.env` есть `STORAGE_METRICS_TOKEN`; скрейп Prometheus работает
- [ ] v1.5.0: `docker compose pull` образов server и console `v1.5.0` (не собирайте их); удалите старые `.env.erasure` / `.env.site-a`, если лаборатории должны взять новые порты; оставьте `STORAGE_KAFKA_BROKERS` пустым, если внешняя лента не нужна
- [ ] v1.6.0: обновите каждый узел до включения шифрования. 409 покрывает лидера 1.5.0 и живого последователя 1.5.0 (попытка аренды; неизвестная версия считается старой). `cluster.nodes` не нужен. Лидер 1.5.0 сам принимает `PutBucketEncryption`, если запрос пришёл на этот процесс. На Postgres или HA отметьте в консоли «Все узлы обновлены до 1.6.0». У S3 `PutBucketEncryption` отметки нет.
- [ ] v1.6.0: не задавайте Vault, если он не нужен. Клиент стартует только когда заданы и `STORAGE_KMS_PROVIDER=vault-transit`, и адрес. SSE-S3 в Vault не ходит. См. [kms-vault.md](kms-vault.md).
