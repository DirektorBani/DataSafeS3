**[English](../en/trusted-clusters.md)** | Русский

# Доверенные кластеры (руководство по консоли)

Пошаговые действия для администратора. Архитектура, env и Docker lab — [Operations — trusted clusters](../../operations-guide/ru/trusted-clusters.md).

> Доступно только **администратору** системы.

## Где в меню

| Раздел | Назначение |
|--------|------------|
| **Кластеры** | Локальный кластер, trusted remote, pairing, правила репликации, revoke |
| **Федерация** | Read proxy к peer S3 — каждый peer привязан к **кластеру** |
| **Site replication** | Классическая репликация по access key (legacy / lab) |

## Страница «Кластеры»

| Блок | Содержимое |
|------|------------|
| **Локальный кластер** | Id, endpoint, генерация join-токена |
| **Доверенные удалённые** | Paired площадки — статус, срок cert, safety number |
| **Присоединиться** | Форма joiner (URL инициатора + токен) |

Узлы HA (leader, standby) при `STORAGE_HA_ENABLED=true` — это **внутри** одного кластера, не trusted remote.

## Добавить удалённую площадку (pairing)

Нужен admin на **обеих** сторонах.

### Инициатор (Site A)

1. **Кластеры** → **Сгенерировать join-токен**.
2. Скопировать `dsjoin_…` сразу — **повторно не показывается**.
3. Передать admin Site B (15 мин, одно использование).

### Joiner (Site B)

1. **Кластеры** → **Присоединиться к кластеру**.
2. Указать URL инициатора, токен, имя (опционально).
3. **Подключить и доверять**.

При успехе обе консоли показывают peer со статусом **healthy**.

### Ошибки pairing

| Ситуация | Проверить |
|----------|-----------|
| Токен недействителен | Новый токен на инициаторе |
| CA verification failed | URL, TLS, MITM |
| Connection error | Firewall, `STORAGE_CLUSTER_ENDPOINT` |
| 403 | Роль administrator |

Ручного подтверждения отпечатка **нет** — trust через mTLS автоматически.

## Карточка remote-кластера

| Поле | Смысл |
|------|-------|
| **Status** | healthy / renewing / revoked |
| **Cert expires** | Срок client cert (авто-renew ~75 дней до expiry) |
| **Safety number** | Только для аудита/поддержки |
| **Правила репликации** | Маппинг bucket → remote bucket |

### Правило репликации

1. Создать source и dest buckets на обеих сторонах.
2. На карточке remote → **Добавить правило**.
3. Загрузить тестовый файл — проверить на remote через несколько секунд.

### Revoke

**Revoke cluster** — немедленная остановка repl и health-check к peer **на вашей** стороне. Повторное подключение — новый join-токен.

## Федерация и кластеры

**Федерация** — read proxy (GetObject / ListObjectsV2). У каждого peer поле **Кластер**:

| Выбор | Когда |
|-------|-------|
| **Локальный** | Peer для reads этой площадки |
| **Remote trusted** | Peer в scope уже paired кластера |

1. **Федерация** → **Зарегистрировать кластер**.
2. Имя, endpoint, region.
3. Выбрать **Кластер** в dropdown.
4. Сохранить.

Federation **не заменяет** trusted replication для DR-записи.

## Шпаргалка

| Задача | Раздел |
|--------|--------|
| Копия в AWS/MinIO | **Gateway** |
| DR на другой DataSafeS3 с mTLS | **Кластеры** + правило |
| Lab по AK/SK | **Site replication** |
| Cross-site read без полной repl | **Федерация** |

## API

```http
GET  /api/v1/clusters
POST /api/v1/clusters/pairing-codes
POST /api/v1/clusters/pair/join
POST /api/v1/clusters/{id}/revoke
GET  /api/v1/clusters/{id}/replication-rules
POST /api/v1/clusters/{id}/replication-rules
```

Схема: [openapi-full.yaml](../../api/openapi-full.yaml).

## См. также

- [Репликация](replication.md)
- [Operations — trusted clusters](../../operations-guide/ru/trusted-clusters.md)
- [User guide — federation & cluster](../../ru/user-guide/08-federation-i-cluster.md)
