# Изображения

English: [images/README.md](../../images/README.md).

## Снимки интерфейса (v1.7.0)

Все кадры сняты на реальной установке DataSafeS3 v1.7.0 из опубликованных образов GitVerse, при работающих интеграциях (LDAP, Keycloak/OIDC, Kafka, NATS, вебхуки, Vault, Grafana, Prometheus, второй сайт и источник MinIO) и вымышленных демо-данных. Одни и те же файлы использует README, документация и сайт проекта; список с подписями и SHA-256 — в [`screenshots/manifest.json`](../../images/screenshots/manifest.json).

Обзор с подписями: [Как это выглядит](../screenshots-gallery.md).

Структура: `screenshots/ru/` (интерфейс на русском), `screenshots/en/` (на английском), `screenshots/common/` (не зависят от языка: вывод терминала и внешние сервисы). Варианты светлой темы оканчиваются на `-light`.

| Файл | Раздел | Подпись |
|------|--------|---------|
| [common/install-clone.png](../../images/screenshots/common/install-clone.png) | Установка | Клонирование репозитория и запуск install.ps1: проверка Docker, портов и выбор профилей (вывод терминала, Windows 11 + Docker Desktop). |
| [common/install-identity.png](../../images/screenshots/common/install-identity.png) | Установка | Установщик поднимает тестовые LDAP и Keycloak и печатает адреса и учётные данные лабораторного стенда (вывод терминала). |
| [common/install-compose.png](../../images/screenshots/common/install-compose.png) | Установка | docker compose up по опубликованным образам GitVerse и проверка готовности консоли (вывод терминала). |
| [common/compose-ps-core.png](../../images/screenshots/common/compose-ps-core.png) | Установка | Базовая установка: сервер хранения, Caddy, PostgreSQL, Prometheus, Grafana, Kafka — все контейнеры запущены (docker compose ps). |
| [common/compose-ps-full.png](../../images/screenshots/common/compose-ps-full.png) | Установка | Полная установка со всеми интеграциями: два узла хранения, Kafka и Kafka UI, NATS, Vault, MinIO-источник, второй сайт, просмотрщик вебхуков (docker ps). |
| [ru/setup-password.png](../../images/screenshots/ru/setup-password.png) | Мастер первоначальной настройки | Мастер настройки, шаг 1: смена пароля администратора при первом входе. |
| [ru/setup-welcome.png](../../images/screenshots/ru/setup-welcome.png) | Мастер первоначальной настройки | Мастер настройки, шаг 2: приветствие, можно подключить внешнее S3-хранилище или пропустить шаг. |
| [ru/setup-s3.png](../../images/screenshots/ru/setup-s3.png) | Мастер первоначальной настройки | Мастер настройки, шаг 3: параметры подключения к внешнему S3 и проверка соединения. |
| [ru/setup-done.png](../../images/screenshots/ru/setup-done.png) | Мастер первоначальной настройки | Мастер настройки, шаг 4: настройка завершена, переход в консоль. |
| [ru/login.png](../../images/screenshots/ru/login.png) | Консоль: обзор, бакеты и объекты | Страница входа в консоль. Переключатель языка вверху справа; доступен вход через SSO. |
| [ru/dashboard.png](../../images/screenshots/ru/dashboard.png) | Консоль: обзор, бакеты и объекты | Обзор: число бакетов, объём, объекты, ключи доступа и состояние API (тёмная тема, демо-данные). |
| [ru/dashboard-light.png](../../images/screenshots/ru/dashboard-light.png) | Консоль: обзор, бакеты и объекты | Обзор в светлой теме. |
| [ru/buckets.png](../../images/screenshots/ru/buckets.png) | Консоль: обзор, бакеты и объекты | Список бакетов: демонстрационные бакеты с разным уровнем доступа (тёмная тема). |
| [ru/buckets-light.png](../../images/screenshots/ru/buckets-light.png) | Консоль: обзор, бакеты и объекты | Список бакетов в светлой теме. |
| [ru/object-browser.png](../../images/screenshots/ru/object-browser.png) | Консоль: обзор, бакеты и объекты | Обозреватель объектов: папки, загрузка, ссылки и панель метаданных выбранного объекта (тёмная тема). |
| [ru/object-browser-light.png](../../images/screenshots/ru/object-browser-light.png) | Консоль: обзор, бакеты и объекты | Обозреватель объектов в светлой теме. |
| [ru/object-details.png](../../images/screenshots/ru/object-details.png) | Консоль: обзор, бакеты и объекты | Бакет с изображениями: список объектов и панель сведений (ключ, размер, тип, теги). |
| [ru/object-versions.png](../../images/screenshots/ru/object-versions.png) | Консоль: обзор, бакеты и объекты | Версии объектов: у каждой версии есть идентификатор, размер и дата; можно скачать или удалить версию. |
| [ru/bucket-versioning.png](../../images/screenshots/ru/bucket-versioning.png) | Консоль: обзор, бакеты и объекты | Настройки бакета, вкладка «Версионирование»: включение версий объектов. |
| [ru/bucket-lifecycle.png](../../images/screenshots/ru/bucket-lifecycle.png) | Консоль: обзор, бакеты и объекты | Правила жизненного цикла бакета: автоматическое удаление объектов по префиксу через заданное число дней. |
| [ru/bucket-access.png](../../images/screenshots/ru/bucket-access.png) | Консоль: обзор, бакеты и объекты | Настройки бакета, вкладка «Доступ»: доступ тенанта и доступ к папкам (префиксам) внутри бакета. |
| [ru/bucket-sharing.png](../../images/screenshots/ru/bucket-sharing.png) | Консоль: обзор, бакеты и объекты | Настройки бакета, вкладка «Обмен файлами»: публичный доступ, ссылки и их срок действия. |
| [ru/bucket-encryption.png](../../images/screenshots/ru/bucket-encryption.png) | Консоль: обзор, бакеты и объекты | Шифрование бакета: ключ сервера или ключ Vault Transit (бакет secure-vault). |
| [ru/keys.png](../../images/screenshots/ru/keys.png) | Доступ, пользователи и аудит | Ключи доступа S3 и API-токены; значения секретных ключей не показываются. |
| [ru/usage.png](../../images/screenshots/ru/usage.png) | Доступ, пользователи и аудит | Использование: объём и число объектов по бакетам, рост хранилища. |
| [ru/users.png](../../images/screenshots/ru/users.png) | Доступ, пользователи и аудит | Пользователи: роли, квоты, статусы и последний вход (вымышленные пользователи). |
| [ru/teams.png](../../images/screenshots/ru/teams.png) | Доступ, пользователи и аудит | Команды: группировка пользователей для выдачи доступа. |
| [ru/team-members.png](../../images/screenshots/ru/team-members.png) | Доступ, пользователи и аудит | Состав выбранной команды: выбор участников. |
| [ru/tenants.png](../../images/screenshots/ru/tenants.png) | Доступ, пользователи и аудит | Тенанты: изолированные организации со своими пользователями и бакетами. |
| [ru/tenant-members.png](../../images/screenshots/ru/tenant-members.png) | Доступ, пользователи и аудит | Тенант: участники и создание пользователей внутри тенанта. |
| [ru/tenant-groups.png](../../images/screenshots/ru/tenant-groups.png) | Доступ, пользователи и аудит | Тенант: группы и привязка к внешним группам (IdP). |
| [ru/activity.png](../../images/screenshots/ru/activity.png) | Доступ, пользователи и аудит | Журнал активности: входы, действия администраторов, события доставки (фильтры и экспорт CSV/JSON). |
| [ru/webhooks.png](../../images/screenshots/ru/webhooks.png) | Доступ, пользователи и аудит | Вебхуки: подписки на события объектов (подключён просмотрщик доставок). |
| [ru/profile.png](../../images/screenshots/ru/profile.png) | Доступ, пользователи и аудит | Профиль: двухфакторная аутентификация (TOTP), Passkeys (WebAuthn) и смена пароля. |
| [ru/settings.png](../../images/screenshots/ru/settings.png) | Настройки и интеграции | Настройки администратора: мягкое удаление, LDAP/AD и OIDC/SSO. |
| [ru/settings-ldap.png](../../images/screenshots/ru/settings-ldap.png) | Настройки и интеграции | Подключение LDAP и OIDC (Keycloak): адреса, учётная запись привязки (пароль скрыт), проверка подключения и синхронизация пользователей. |
| [ru/settings-sinks.png](../../images/screenshots/ru/settings-sinks.png) | Настройки и интеграции | Внешнее логирование: приёмники журнала (Webhook подключён к демо-просмотрщику). |
| [ru/settings-buckets.png](../../images/screenshots/ru/settings-buckets.png) | Настройки и интеграции | Настройки бакетов: массовое применение параметров к нескольким бакетам. |
| [ru/settings-security.png](../../images/screenshots/ru/settings-security.png) | Настройки и интеграции | Безопасность: публичный доступ, предподписанные ссылки, сроки действия и MFA. |
| [ru/settings-events.png](../../images/screenshots/ru/settings-events.png) | Настройки и интеграции | Доставка событий: Kafka, NATS JetStream и вебхуки — очередь и число доставленных событий. |
| [ru/settings-kms.png](../../images/screenshots/ru/settings-kms.png) | Настройки и интеграции | Vault (KMS): подключение к Vault Transit для шифрования; токен не показывается. |
| [ru/settings-migrate.png](../../images/screenshots/ru/settings-migrate.png) | Настройки и интеграции | Миграция из MinIO: мастер формирует команду для утилиты миграции. |
| [ru/settings-migrate-filled.png](../../images/screenshots/ru/settings-migrate-filled.png) | Настройки и интеграции | Миграция из MinIO: параметры источника и получателя заполнены, команда готова. |
| [ru/settings-migrate-iam.png](../../images/screenshots/ru/settings-migrate-iam.png) | Настройки и интеграции | Миграция из MinIO: перенос IAM (пользователи, группы, политики). |
| [ru/gateway.png](../../images/screenshots/ru/gateway.png) | Шлюз, репликация и кластер | Шлюз: подключения к внешнему S3-хранилищу (демо-площадка Site B) с проверкой соединения и форма добавления подключения. |
| [ru/gateway-rules.png](../../images/screenshots/ru/gateway-rules.png) | Шлюз, репликация и кластер | Шлюз: правила репликации и ручной запуск синхронизации. |
| [ru/gateway-jobs.png](../../images/screenshots/ru/gateway-jobs.png) | Шлюз, репликация и кластер | Шлюз: задачи синхронизации и их результат. |
| [ru/gateway-status.png](../../images/screenshots/ru/gateway-status.png) | Шлюз, репликация и кластер | Шлюз: состояние — подключения 2/2 OK, одно правило, очередь репликации пуста, реплицировано 4 задачи. |
| [ru/cluster.png](../../images/screenshots/ru/cluster.png) | Шлюз, репликация и кластер | Кластеры: локальный узел и доверенные кластеры. |
| [common/ha-status.png](../../images/screenshots/common/ha-status.png) | Шлюз, репликация и кластер | HA из двух узлов: ответ /cluster/status — режим включён, выбран лидер (вывод терминала). |
| [common/monitoring.png](../../images/screenshots/common/monitoring.png) | Мониторинг и внешние сервисы | Grafana, дашборд обзора: запросы, объём, число объектов, задержки и ошибки HTTP (данные из Prometheus). |
| [common/grafana-buckets.png](../../images/screenshots/common/grafana-buckets.png) | Мониторинг и внешние сервисы | Grafana, дашборд бакетов: объём и число объектов по бакетам. |
| [common/prometheus-targets.png](../../images/screenshots/common/prometheus-targets.png) | Мониторинг и внешние сервисы | Prometheus: цель сбора метрик storage-server в состоянии UP. |
| [common/kafka-ui-topics.png](../../images/screenshots/common/kafka-ui-topics.png) | Мониторинг и внешние сервисы | Kafka UI: тема datasafe.events с событиями хранилища. |
| [common/kafka-ui-messages.png](../../images/screenshots/common/kafka-ui-messages.png) | Мониторинг и внешние сервисы | Kafka UI: сообщения темы datasafe.events — события создания и удаления объектов. |
| [common/webhook-viewer.png](../../images/screenshots/common/webhook-viewer.png) | Мониторинг и внешние сервисы | Просмотрщик вебхуков: полученные доставки событий (демо-приёмник на localhost). |
| [common/nats.png](../../images/screenshots/common/nats.png) | Мониторинг и внешние сервисы | NATS JetStream: поток DATASAFE_EVENTS с сообщениями событий (вывод терминала). |
| [common/vault.png](../../images/screenshots/common/vault.png) | Мониторинг и внешние сервисы | Vault Transit: ключ шифрования бакета и статус Vault (вывод терминала, токен скрыт). |
| [common/minio-src.png](../../images/screenshots/common/minio-src.png) | Мониторинг и внешние сервисы | MinIO как источник миграции: список объектов тестового бакета (вывод терминала). |
| [common/keycloak-login.png](../../images/screenshots/common/keycloak-login.png) | Вход через LDAP и OIDC (Keycloak) | Keycloak: страница входа realm datasafe при входе в консоль через OIDC. |
| [ru/sso-profile.png](../../images/screenshots/ru/sso-profile.png) | Вход через LDAP и OIDC (Keycloak) | Профиль пользователя, вошедшего через OIDC (Keycloak). |
| [ru/ldap-profile.png](../../images/screenshots/ru/ldap-profile.png) | Вход через LDAP и OIDC (Keycloak) | Профиль пользователя, вошедшего через LDAP. |
| [ru/ldap-buckets.png](../../images/screenshots/ru/ldap-buckets.png) | Вход через LDAP и OIDC (Keycloak) | Личное хранилище пользователя LDAP: вошедший пользователь видит только свой бакет files. |

Набор не пересоздаётся скриптами: прежние скрипты съёмки (`scripts/capture-screenshots.mjs`, `scripts/screenshots/capture.mjs`) пишут в `docs/images/_capture-output/` и не затирают опубликованные файлы.

## Диаграммы

Диаграммы архитектуры — Mermaid в Markdown. См. [diagrams/README.md](../diagrams/README.md).
