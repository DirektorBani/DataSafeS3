**[English](../en/kms-vault.md)** | Русский

# Vault Transit (ключи объектов)

Это необязательное шифрование объектов. Это не [подстановка переменных окружения](secrets-vault.md). Пока адрес не задан, процесс не открывает соединение с Vault, не запускает опрос и не регистрирует метрики KMS.

SSE-S3 (`AES256`) всегда использует `STORAGE_SSE_MASTER_KEY` и в Vault не ходит. Бакет без правила по умолчанию и запрос без заголовка шифрования остаются открытым текстом.

`aws:kms` ходит в Vault только если провайдер `vault-transit` и задан адрес (переменная окружения или **Настройки → Vault (KMS)**). До этого `aws:kms` по-прежнему заворачивает ключ сервера, а в метаданных провайдер остаётся `local`.

## Когда появляется клиент

Нужны оба условия. Каждое поле можно задать окружением или консолью. Значение из окружения важнее: поле в консоли показывает **Задано переменной окружения**.

| Переменная | По умолчанию | Назначение |
|------------|--------------|------------|
| `STORAGE_KMS_PROVIDER` | пусто | `vault-transit` включает клиент. Пустое и любое другое значение его не создают. |
| `STORAGE_KMS_VAULT_ADDR` | пусто | Адрес Vault. Сохранённый в консоли адрес действует, пока переменная пуста. |
| `STORAGE_KMS_VAULT_NAMESPACE` | пусто | Namespace Vault Enterprise. |
| `STORAGE_KMS_VAULT_TRANSIT_MOUNT` | `transit` | Путь Transit. |
| `STORAGE_KMS_VAULT_CACERT` | пусто | PEM или путь к PEM-файлу на этом узле. |
| `STORAGE_KMS_VAULT_ROLE_ID` | пусто | Role ID AppRole этого узла. |
| `STORAGE_KMS_VAULT_SECRET_ID` или `_SECRET_ID_FILE` | пусто | Secret ID. Значение узла перекрывает секрет из общих метаданных. |
| `STORAGE_KMS_VAULT_TOKEN_FILE` | пусто | Файл токена вместо AppRole. Консоль может хранить токен; из окружения читается файл. Токен, который Vault не продлевает (корневой токен dev-режима отвечает 400 `token not renewable`), всё равно используется. HTTP 403 и любой другой 400 токен отклоняют. |
| `STORAGE_KMS_VAULT_ADMIN_ROLE_ID` / `_SECRET_ID` / `_SECRET_ID_FILE` | пусто | Отдельный AppRole для rotate и delete. |
| `STORAGE_KMS_DEFAULT_KEY` | `datasafe-<tenant>-<bucket>` | Имя ключа по умолчанию. |
| `STORAGE_KMS_KEY_ALLOWLIST` | `datasafe-*` | Имена, которые может запросить клиент. |
| `STORAGE_KMS_DEK_CACHE_TTL` | `5m` | Сколько развёрнутый ключ данных живёт в памяти. |
| `STORAGE_KMS_POLL_INTERVAL` | `2s` | Как часто узел читает `kms_keys.generation` и сбрасывает устаревший кэш. |
| `STORAGE_KMS_TIMEOUT` | `5s` | Один запрос к Vault. |

Клиент — `net/http`. SDK Vault в сервер не входит. Узел звонит в Vault только когда заданы и провайдер, и адрес. Общие настройки лежат в метаданных (`system_config.kms`). Secret ID и токен шифруются как поля. `GET /api/v1/kms/settings`, `/healthz`, Activity и журналы эти секреты не возвращают. Сохранённый секрет показан как заданный; поле принимает только замену.

У каждого узла может быть свой AppRole в окружении. Этот секрет обратно в общую конфигурацию не пишется.

Лабораторный оверлей (dev-режим, корневой токен, не для production): `deploy/compose/docker-compose.vault-kms.yml`. Политики: `deploy/vault/policies/datasafe-kms.hcl` (encrypt, decrypt, rewrap, создание и чтение `datasafe-*`) и `datasafe-kms-admin.hcl` (rotate, delete и флаг удаления).

## Когда Vault недоступен

Новый объект SSE-KMS заворачивается до записи. Если Vault недоступен и ключа данных нет в кэше, запись отвечает HTTP 503 `KMS unavailable`, и объект не сохраняется. Чтение SSE-KMS без кэша — так же. Попадание в кэш в пределах TTL расшифровывает. Открытые бакеты и SSE-S3 в Vault не ходят.

Ссылка общего доступа на объект SSE-KMS отвечает HTTP 503 с текстом `Файл временно недоступен, попробуйте позже`. Ссылка на открытый объект не меняется.

`/healthz` добавляет `kms.provider` и `kms.available`. Этот запрос сам Vault не опрашивает. Gauge `datasafe_kms_available` публикуется только пока Vault включён. Алерт `DataSafeKMSUnavailable` в `deploy/docker/prometheus/alerts.yml` срабатывает, если gauge равен 0 дольше 1 минуты.

Activity пишет `kms_unavailable` и `kms_recovered` только при смене состояния. Ресурс — id узла. Секрета в строке нет.

Страница консоли — **Настройки → Vault (KMS)** (`/admin/settings/kms`). Кнопка **Проверить подключение** вызывает `POST /api/v1/kms/test`. После сохранения адреса карточка бакета включает **Ключ из Vault**. Таблица узлов читает heartbeat за последние 10 минут (миграция Postgres `026`).

Задачи rewrap и crypto-shred в этот шаг не входят.
