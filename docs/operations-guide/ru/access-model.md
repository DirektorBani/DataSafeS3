# Модель доступа

Кто может читать, записывать, обмениваться файлами и администрировать бакет. Таблица ниже совпадает с `internal/auth/access_matrix.go`. Тест падает, если этот раздел или английский гайд расходятся с кодом.

Значения ячеек:

- `allow` — роль может выполнить действие.
- `deny` — не может.
- `write-grant` — только при гранте на запись в этот бакет. Оператор не получает запись во все бакеты.
- `compat-flag` — только при `STORAGE_OPERATOR_WRITE_ALL=true`. Переменная устарела и будет удалена в 1.7.0. При старте процесс пишет предупреждение в журнал, Activity `operator_write_all_enabled` и баннер в консоли для глобальных администраторов.

Администратор бакета — владелец, администратор тенанта этого бакета, глобальный администратор или грант с уровнем `admin`. Гранты чтения и записи не включают очистку корзины, retention COMPLIANCE, изменение грантов и политику обмена.

Анонимное чтение разрешено только для одного однозначного публичного бакета (заголовок `X-DataSafe-Tenant` или query `tenant`, если имена совпали) и только если Block Public Access выключен на всех уровнях. `public-read` хранится как политика бакета с `Principal: "*"`.

Presigned-ссылки подписываются временным ключом вызывающего пользователя. Корневой ключ для presign не используется. DELETE подписать нельзя. Срок не больше 7 дней и не больше эффективного `max_presign_ttl`.

STS `AssumeRole` отклоняет непустой `RoleArn`. Сессия наследует права вызывающего. В ответе ARN — ARN пользователя, а не значение из запроса.

`POST /api/v1/clusters/{id}/rotate` и `/revoke-notify` требуют клиентский сертификат, который этот кластер выдал активному доверенному пиру (TLS-сертификат клиента или заголовки `X-DataSafe-Cluster-Cert` и `X-DataSafe-Cluster-Sig` на обычном HTTP). Запрос без этого доказательства — `401`.

## Запись в кластере

Изменения обмена и настроек безопасности — обычные записи. При `STORAGE_HA_ENABLED=true` и метаданных в Postgres follower отвечает **503** `not cluster leader` и `Retry-After: 30`, как с v1.1. Запись нужно отправлять на лидера или на балансировщик, который ведёт запись на лидера. Это относится к:

- `PUT /api/v1/settings/sharing`, `PUT /api/v1/tenants/{id}/sharing`, `PUT /api/v1/buckets/{bucket}/sharing`
- `POST /api/v1/buckets/{bucket}/shares` и `POST .../shares/revoke-all`
- `DELETE /api/v1/shares/{id}`
- presign, потому что он создаёт временный ключ доступа

`GET /api/v1/public/share/{token}` и скачивание по ссылке на follower разрешены. Первый отказ после истечения ссылки или после исчерпания `max_downloads` порождает `ShareExpired` или `ShareLimitReached` ровно один раз (одна строка Activity `share.expired` или `share.limit_reached` и одна строка outbox). Следующие отказы той же ссылки событие не создают. `ShareBlocked` пишется на каждую попытку скачивания, пока обмен выключен. Follower кладёт эту строку в outbox, даже если локально sink webhook ещё не зарегистрирован; доставляет её лидер.

<!-- access-matrix:begin -->
| Action | Global admin | Operator | Tenant admin | Bucket admin | Write | Read | Anonymous |
| --- | --- | --- | --- | --- | --- | --- | --- |
| presign GET | allow | allow | allow | allow | allow | allow | deny |
| presign PUT | allow | write-grant | allow | allow | allow | deny | deny |
| presign DELETE | deny | deny | deny | deny | deny | deny | deny |
| create public-read bucket | allow | deny | allow | deny | deny | deny | deny |
| set public-read after create | allow | deny | allow | allow | deny | deny | deny |
| anonymous GET when public and not blocked | allow | allow | allow | allow | allow | allow | allow |
| anonymous GET when block public access | deny | deny | deny | deny | deny | deny | deny |
| list own share links | allow | allow | allow | allow | allow | deny | deny |
| list every share link | allow | deny | allow | allow | deny | deny | deny |
| create share link | allow | write-grant | allow | allow | allow | deny | deny |
| revoke own share link | allow | write-grant | allow | allow | allow | deny | deny |
| revoke another user's share link | allow | deny | allow | allow | deny | deny | deny |
| revoke all share links | allow | deny | allow | allow | deny | deny | deny |
| change bucket sharing policy | allow | deny | allow | allow | deny | deny | deny |
| change tenant sharing policy | allow | deny | allow | deny | deny | deny | deny |
| change global sharing policy | allow | deny | deny | deny | deny | deny | deny |
| legal hold | allow | write-grant | allow | allow | allow | deny | deny |
| retention GOVERNANCE | allow | write-grant | allow | allow | allow | deny | deny |
| retention COMPLIANCE | allow | deny | allow | allow | deny | deny | deny |
| change storage class | allow | write-grant | allow | allow | allow | deny | deny |
| restore from trash | allow | write-grant | allow | allow | allow | deny | deny |
| purge trash | allow | deny | allow | allow | deny | deny | deny |
| change bucket grants | allow | deny | allow | allow | deny | deny | deny |
| delete bucket | allow | compat-flag | allow | allow | deny | deny | deny |
| operator write without a grant | deny | deny | deny | deny | deny | deny | deny |
| cluster rotate or revoke-notify without a peer certificate | deny | deny | deny | deny | deny | deny | deny |
| STS RoleArn echoed unchecked | deny | deny | deny | deny | deny | deny | deny |
<!-- access-matrix:end -->
