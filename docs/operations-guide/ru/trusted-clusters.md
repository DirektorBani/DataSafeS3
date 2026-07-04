**[English](../en/trusted-clusters.md)** | Русский

# Доверенные кластеры (multi-site trust)

Как связать **несколько инсталляций** DataSafeS3 для **автоматического mTLS** и **репликации объектов** — без хранения долгоживущих паролей в БД в открытом виде.

> **Статус:** входит в **v1.1.0** как CE lab foundation. Проверьте pairing, backup cert и репликацию в своей сети перед production.

## Зачем это нужно

Несколько площадок (офис + DR, два ЦОД, lab Site A + Site B). Нужно:

1. **Доверие** — удалённая площадка действительно ваша, не MITM.
2. **Репликация** — копирование bucket/объектов после PUT/DELETE.
3. **Опционально federation** — прокси GetObject/List **в привязке к кластеру**.

Доверенные кластеры закрывают (1) и (2). **Федерация** (Admin → Федерация) — пункт (3); при регистрации peer выбирается **кластер**.

## Термины

| Термин | Значение |
|--------|----------|
| **Локальный кластер** | Эта инсталляция (`STORAGE_CLUSTER_ID`, по умолчанию `local`). |
| **Доверенный удалённый кластер** | Площадка после pairing — health-check по mTLS. |
| **Join-токен** (`dsjoin_*`) | Одноразовый код (15 мин). Показывается один раз; в БД только hash. |
| **Pairing** | Обмен CA и подпись client cert — **без** ручного «подтвердите отпечаток». |
| **Safety number** | Короткий код для аудита/поддержки, не gate безопасности. |
| **Правило репликации** | `source_bucket` локально → `dest_bucket` на remote. |

### Три функции — не путать

| Функция | Направление | Аутентификация | Консоль |
|---------|-------------|----------------|---------|
| **Gateway replication** | В **любой** S3 | Access key + secret | Gateway |
| **Site replication (classic)** | На другой DataSafeS3 | AK/SK на peer | Site replication |
| **Trusted cluster replication** | На **paired** DataSafeS3 | mTLS + локальный S3 SigV4 | Кластеры |
| **Federation** | Read proxy (Get/List) | Реестр peer | Федерация (+ **кластер**) |

Для DR между двумя DataSafeS3 предпочитайте **trusted cluster replication**. **Gateway** — для MinIO/AWS. **Federation** — когда нужны только cross-site чтения.

## Безопасность (для оператора)

| Контроль | Поведение |
|----------|-----------|
| Транспорт | TLS 1.3; HTTP только при `STORAGE_DEV=true` (lab). |
| Pairing | mTLS + hash токена; неверный CA → fail + audit. |
| Секреты в БД | Hash или `enc:v1:`, не plaintext. |
| TTL client cert | 90 дней; **авто-обновление** ~75 дней (rotator на leader). |
| Revoke | CRL + остановка workers. |
| Private keys | На диске (`STORAGE_CLUSTER_CERT_DIR`), не в Postgres. |

Делайте backup каталога `cluster-certs` как для TLS-материала.

## Переменные окружения

| Переменная | По умолчанию | Назначение |
|------------|--------------|------------|
| `STORAGE_CLUSTER_ID` | `local` | Id этой площадки. |
| `STORAGE_CLUSTER_ENDPOINT` | Из Host запроса | URL для pairing/health **с другой площадки**. |
| `STORAGE_CLUSTER_CERT_DIR` | `{data}/cluster-certs` | CA и ключи. |
| `STORAGE_CLUSTER_CERT_RENEW_BEFORE_DAYS` | `75` | Обновление до истечения 90d. |
| `STORAGE_TRUSTED_CLUSTER_REPL_ENABLED` | `true` | Trusted-cluster repl rules. |
| `STORAGE_DEV` | `false` в prod | HTTP и private IP в lab. |

**Важно:** endpoint должен быть **достижим с remote-контейнера**. В Docker Desktop: `http://host.docker.internal:9002`, не `127.0.0.1`.

## Pairing (операции)

### Инициатор (Site A)

1. Задать `STORAGE_CLUSTER_ID` и `STORAGE_CLUSTER_ENDPOINT`.
2. Перезапустить `storage-server`.
3. Консоль → **Кластеры** → **Сгенерировать join-токен** → скопировать `dsjoin_*`.

### Joiner (Site B)

1. Свои `STORAGE_CLUSTER_ID` и `STORAGE_CLUSTER_ENDPOINT`.
2. Консоль → **Кластеры** → URL инициатора + токен → **Подключить**.
3. API: `POST /api/v1/clusters/pair/join`.

### Репликация после pairing

1. Создать buckets на обеих сторонах.
2. На инициаторе: карточка remote → **Добавить правило репликации**.
3. PUT объекта → через ~2 с на remote.
4. Lag: `GET /api/v1/site-replication/status`.

### Revoke

Консоль → remote → **Revoke**. Репликация и health-check прекращаются.

## Windows lab

```powershell
cd D:\cursor_p
$env:GOOS='linux'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
go build -trimpath -o deploy/docker/storage-server-linux ./cmd/storage-server
powershell -File scripts\ha\start-ha-stack.ps1 -SkipBuild
powershell -File scripts\ha\start-ha-cluster-b.ps1 -SkipBuild
powershell -File scripts\ha\test-trusted-cluster-pairing.ps1
```

Пример `.env` — см. [английскую версию](../en/trusted-clusters.md#windows-lab-ha--cluster-b).

## Балансировка внутри одного кластера

Write → leader, read → любой healthy node: [multi-cluster-lb.md](../../../deploy/caddy/multi-cluster-lb.md).

Cross-site mTLS **не** терминируйте на edge LB.

## Миграции Postgres

| Версия | Содержимое |
|--------|------------|
| `017` | trusted_clusters, pairing, certificates |
| `018` | federation `cluster_id` |
| `019` | site_repl `trusted_cluster_id` |

См. [upgrade](./upgrade.md#trusted-clusters-post-v110).

## Troubleshooting

| Симптом | Причина | Решение |
|---------|---------|---------|
| Remote **unhealthy** | `127.0.0.1` в Docker | `host.docker.internal`; re-pair |
| Pairing **401** | Истёк/использован токен | Новый `dsjoin_*` |
| Объекты не реплицируются | Нет rule / unhealthy peer | Правило + env repl enabled |

## Связанные материалы

- [Репликация (admin)](../../administrator-guide/ru/trusted-clusters.md)
- [Масштабирование](./scaling.md)
- [Спека](../../specs/multi-cluster-trusted-replication-tz.md)
