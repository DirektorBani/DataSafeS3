# Модель доступа

Кто может читать, записывать, обмениваться файлами и администрировать бакет. Таблица ниже совпадает с `internal/auth/access_matrix.go`. Тест падает, если этот раздел или английский гайд расходятся с кодом.

Значения ячеек:

- `allow` — роль может выполнить действие.
- `deny` — не может.
- `write-grant` — только при гранте на запись в этот бакет. Оператор не получает запись во все бакеты.
- `compat-flag` — только при `STORAGE_OPERATOR_WRITE_ALL=true`. Переменная устарела и будет удалена в 1.7.0. При старте процесс пишет предупреждение в журнал, Activity `operator_write_all_enabled` и баннер в консоли для глобальных администраторов.

Администратор бакета — владелец, администратор тенанта этого бакета, глобальный администратор или грант с уровнем `admin`. Гранты чтения и записи не включают очистку корзины, retention COMPLIANCE, изменение грантов и политику обмена.

`aws:SourceIp` в политике бакета — TCP-адрес того, кто подключился к этому процессу. За балансировщиком это адрес балансировщика, а клиентский `X-Forwarded-For` игнорируется. `STORAGE_TRUSTED_PROXIES` — CIDR балансировщиков через запятую; только соединение из этих сетей может передать адрес клиента в `X-Forwarded-For` (самый правый хоп, которого нет в списке). `s3:ListAllMyBuckets` для ключа, который не администратор, не оператор и не корневой ключ, — HTTP 403 AccessDenied, если политика бакета явно не разрешает это действие. Консоль берёт список через `GET /api/v1/buckets`.

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
- `PUT /api/v1/settings/mfa-policy`, `POST /api/v1/auth/step-up`, `POST /api/v1/users/{id}/mfa/reset` и `POST /api/v1/keys`
- `GET/PUT/DELETE /api/v1/buckets/{bucket}/encryption`

`GET /api/v1/public/share/{token}` и скачивание по ссылке на follower разрешены. Первый отказ после истечения ссылки или после исчерпания `max_downloads` порождает `ShareExpired` или `ShareLimitReached` ровно один раз (одна строка Activity `share.expired` или `share.limit_reached` и одна строка outbox). Следующие отказы той же ссылки событие не создают. `ShareBlocked` пишется на каждую попытку скачивания, пока обмен выключен. Follower кладёт эту строку в outbox, даже если локально sink webhook ещё не зарегистрирован; доставляет её лидер.

## Обязательная 2FA для администраторов бакетов

`PUT /api/v1/settings/mfa-policy` (только глобальный администратор) пишет политику в `SystemConfig.MFA`. Каждый узел читает её на следующем запросе. `require_admin_mfa` остаётся и совпадает с `apply_to_global_admins`.

Кого касается при `require_bucket_admin_mfa`: пользователь с грантом `admin` хотя бы на одном бакете, включая владельца; администраторы тенантов, если включён `apply_to_tenant_admins`; глобальные администраторы, если включён `apply_to_global_admins`. Глобальный администратор, который владеет бакетом или имеет грант `admin`, попадает под правило администратора бакета даже когда `apply_to_global_admins` выключен. Сброс 2FA другого пользователя — опасное действие: без своего фактора такой администратор его не выполнит. Достаточно TOTP или ключа безопасности. Вход OIDC засчитывается как 2FA, если `amr` ID-токена содержит `mfa`, `otp`, `hwk`, `swk`, `fpt` (или значение из `oidc_accepted_amr`), либо `acr` есть в `oidc_accepted_acr`. Для LDAP — только локальный фактор. Фактор IdP не подтверждает опасное действие: для этого нужен код TOTP или ключ безопасности DataSafe.

Льготный период (`grace_period_days`, 0–90, по умолчанию 7) считается от включения политики или от получения прав администратора бакета — что позже. В этот срок консоль показывает баннер, а действия ниже разрешены, если фактор ещё не зарегистрирован. После срока они отвечают `403 {"error":"mfa_required","message":"требуется 2FA"}`. Читать и писать объекты по-прежнему можно.

При выключенной политике подтверждение как в 1.5.0. При включённой каждое опасное действие пользователя с локальным фактором требует нового кода, в том числе в льготный период. `POST /api/v1/auth/step-up` с `{action,target,totp_code}` выдаёт одноразовый токен (`STORAGE_MFA_ACTION_TOKEN_TTL`, 30–300 с, по умолчанию 60), привязанный к пользователю, сессии, действию и цели. Его передают в `X-DataSafe-MFA-Token`. Повтор, истечение, другой пользователь, действие или цель — `401 {"error":"mfa_token_invalid"}`. Нет заголовка — `401 {"error":"step_up_required","action":...}`. Один и тот же шаг TOTP принимается один раз: либо вход, либо подтверждение. Пять неудачных подтверждений за пять минут блокируют пользователя на пять минут.

S3 не принимает `X-DataSafe-MFA-Token`. Ключ доступа и STS-сессию пользователя, на которого действует политика, можно создать только из подтверждённой сессии; у ключа `created_with_mfa=true`. `PutBucketEncryption`, `DeleteBucketEncryption`, `PutBucketPolicy`, `DeleteBucketPolicy`, `PutObjectLockConfiguration`, `PutObjectRetention` (COMPLIANCE или более ранний срок), приостановка `PutBucketVersioning` и `DeleteBucket` ключом без флага — `403 AccessDenied` «ключ создан без 2FA; создайте новый ключ после входа с 2FA». `PutObject` выполняется. Корневой ключ сервера без владельца под это правило не попадает.

`MfaRequiredDenied` — не чаще одного события в минуту на пару пользователь и действие. `MfaPolicyChanged`, `MfaStepUpRequired`, `MfaStepUpVerified` и `MfaReset` пишутся в outbox и Activity.

`PUT /api/v1/buckets/{bucket}/encryption` пишет ту же запись, что S3 `PutBucketEncryption`. Включение после льготного периода требует фактора (`403 mfa_required`), но не токена действия. Выключение и `DELETE` — опасное действие `encryption_disable`: без `X-DataSafe-MFA-Token` ответ `401 step_up_required`. Анонимный GET и HEAD объекта с `aws:kms` — `403` «объект зашифрован ключом KMS, публичный доступ невозможен». `public-read` вместе с default `aws:kms` — `409` «Публичный доступ к файлам, зашифрованным ключом из Vault, невозможен. Сначала сделайте бакет приватным». Ссылка и presigned на любой зашифрованный объект (`AES256` и `aws:kms`) снова проверяют, что создатель или владелец ключа активен и может читать. Для незашифрованного объекта повторной проверки нет: ссылка работает до срока или лимита скачиваний. Уничтоженный ключ — `403` «Ключ шифрования уничтожен, файл недоступен навсегда». SSE-S3 (`AES256`) по-прежнему отдаётся анонимно, если публичный доступ не заблокирован.

<!-- dangerous-actions:begin -->
| action | meaning |
| --- | --- |
| crypto_shred | destroy a KMS key (crypto-shred) |
| key_rotate | rotate a KMS key |
| trash_purge | purge trash |
| sharing_policy_change | any sharing-policy change, including turning sharing off |
| grants_change | change bucket grants |
| encryption_disable | turn bucket encryption off |
| object_lock_config | Object Lock configuration |
| compliance_retention | COMPLIANCE retention, including extend or shorten |
| bucket_delete | delete a bucket |
| mfa_reset | reset a user's MFA |
| revoke_all_shares | revoke every share link on a bucket |
| access_key_create | create an access key or STS session for a covered user |
<!-- dangerous-actions:end -->

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
