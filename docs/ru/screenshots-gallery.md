# Как это выглядит

Снимки сделаны на реальной установке DataSafeS3 v1.7.0 из опубликованных образов на GitVerse (Windows 11 + Docker Desktop, `docker compose`) со всеми интеграциями: LDAP, Keycloak (OIDC), Kafka, NATS JetStream, вебхуки, HashiCorp Vault (KMS), Grafana и Prometheus, второй сайт и MinIO-источник. Данные в примерах вымышленные: пользователи, бакеты и файлы созданы для демонстрации, адреса — только `localhost`, секреты и пароли на снимках не показаны. Часть кадров — снимки терминала: они собраны из реального вывода команд. Снимки на русском; если английского варианта нет, экран одинаков в обоих языках интерфейса.

Галерея помогает увидеть интерфейс до установки. Сами экраны — [руководство пользователя](user-guide/README.md) и [руководство администратора](../administrator-guide/ru/README.md).

## Установка

### Клонирование и install.ps1

![DataSafeS3 — Клонирование и install.ps1](../images/screenshots/common/install-clone.png)

*Клонирование репозитория и запуск install.ps1: проверка Docker, портов и выбор профилей (вывод терминала, Windows 11 + Docker Desktop).*

### LDAP и Keycloak

![DataSafeS3 — LDAP и Keycloak](../images/screenshots/common/install-identity.png)

*Установщик поднимает тестовые LDAP и Keycloak и печатает адреса и учётные данные лабораторного стенда (вывод терминала).*

### docker compose up

![DataSafeS3 — docker compose up](../images/screenshots/common/install-compose.png)

*docker compose up по опубликованным образам GitVerse и проверка готовности консоли (вывод терминала).*

### Базовые сервисы

![DataSafeS3 — Базовые сервисы](../images/screenshots/common/compose-ps-core.png)

*Базовая установка: сервер хранения, Caddy, PostgreSQL, Prometheus, Grafana, Kafka — все контейнеры запущены (docker compose ps).*

### Все интеграции

![DataSafeS3 — Все интеграции](../images/screenshots/common/compose-ps-full.png)

*Полная установка со всеми интеграциями: два узла хранения, Kafka и Kafka UI, NATS, Vault, MinIO-источник, второй сайт, просмотрщик вебхуков (docker ps).*

## Мастер первоначальной настройки

### Шаг 1: пароль

![DataSafeS3 — Шаг 1: пароль](../images/screenshots/ru/setup-password.png)

*Мастер настройки, шаг 1: смена пароля администратора при первом входе.*

### Шаг 2: приветствие

![DataSafeS3 — Шаг 2: приветствие](../images/screenshots/ru/setup-welcome.png)

*Мастер настройки, шаг 2: приветствие, можно подключить внешнее S3-хранилище или пропустить шаг.*

### Шаг 3: внешний S3

![DataSafeS3 — Шаг 3: внешний S3](../images/screenshots/ru/setup-s3.png)

*Мастер настройки, шаг 3: параметры подключения к внешнему S3 и проверка соединения.*

### Шаг 4: готово

![DataSafeS3 — Шаг 4: готово](../images/screenshots/ru/setup-done.png)

*Мастер настройки, шаг 4: настройка завершена, переход в консоль.*

## Консоль: обзор, бакеты и объекты

### Вход

![DataSafeS3 — Вход](../images/screenshots/ru/login.png)

*Страница входа в консоль. Переключатель языка вверху справа; доступен вход через SSO.*

### Обзор

![DataSafeS3 — Обзор](../images/screenshots/ru/dashboard.png)

*Обзор: число бакетов, объём, объекты, ключи доступа и состояние API (тёмная тема, демо-данные).*

### Обзор (светлая)

![DataSafeS3 — Обзор (светлая)](../images/screenshots/ru/dashboard-light.png)

*Обзор в светлой теме.*

### Бакеты

![DataSafeS3 — Бакеты](../images/screenshots/ru/buckets.png)

*Список бакетов: демонстрационные бакеты с разным уровнем доступа (тёмная тема).*

### Бакеты (светлая)

![DataSafeS3 — Бакеты (светлая)](../images/screenshots/ru/buckets-light.png)

*Список бакетов в светлой теме.*

### Обозреватель объектов

![DataSafeS3 — Обозреватель объектов](../images/screenshots/ru/object-browser.png)

*Обозреватель объектов: папки, загрузка, ссылки и панель метаданных выбранного объекта (тёмная тема).*

### Объекты (светлая)

![DataSafeS3 — Объекты (светлая)](../images/screenshots/ru/object-browser-light.png)

*Обозреватель объектов в светлой теме.*

### Сведения об объекте

![DataSafeS3 — Сведения об объекте](../images/screenshots/ru/object-details.png)

*Бакет с изображениями: список объектов и панель сведений (ключ, размер, тип, теги).*

### Версии объектов

![DataSafeS3 — Версии объектов](../images/screenshots/ru/object-versions.png)

*Версии объектов: у каждой версии есть идентификатор, размер и дата; можно скачать или удалить версию.*

### Версионирование

![DataSafeS3 — Версионирование](../images/screenshots/ru/bucket-versioning.png)

*Настройки бакета, вкладка «Версионирование»: включение версий объектов.*

### Жизненный цикл

![DataSafeS3 — Жизненный цикл](../images/screenshots/ru/bucket-lifecycle.png)

*Правила жизненного цикла бакета: автоматическое удаление объектов по префиксу через заданное число дней.*

### Доступ к бакету

![DataSafeS3 — Доступ к бакету](../images/screenshots/ru/bucket-access.png)

*Настройки бакета, вкладка «Доступ»: доступ тенанта и доступ к папкам (префиксам) внутри бакета.*

### Обмен файлами

![DataSafeS3 — Обмен файлами](../images/screenshots/ru/bucket-sharing.png)

*Настройки бакета, вкладка «Обмен файлами»: публичный доступ, ссылки и их срок действия.*

### Шифрование бакета

![DataSafeS3 — Шифрование бакета](../images/screenshots/ru/bucket-encryption.png)

*Шифрование бакета: ключ сервера или ключ Vault Transit (бакет secure-vault).*

## Доступ, пользователи и аудит

### Ключи доступа

![DataSafeS3 — Ключи доступа](../images/screenshots/ru/keys.png)

*Ключи доступа S3 и API-токены; значения секретных ключей не показываются.*

### Использование

![DataSafeS3 — Использование](../images/screenshots/ru/usage.png)

*Использование: объём и число объектов по бакетам, рост хранилища.*

### Пользователи

![DataSafeS3 — Пользователи](../images/screenshots/ru/users.png)

*Пользователи: роли, квоты, статусы и последний вход (вымышленные пользователи).*

### Команды

![DataSafeS3 — Команды](../images/screenshots/ru/teams.png)

*Команды: группировка пользователей для выдачи доступа.*

### Состав команды

![DataSafeS3 — Состав команды](../images/screenshots/ru/team-members.png)

*Состав выбранной команды: выбор участников.*

### Тенанты

![DataSafeS3 — Тенанты](../images/screenshots/ru/tenants.png)

*Тенанты: изолированные организации со своими пользователями и бакетами.*

### Участники тенанта

![DataSafeS3 — Участники тенанта](../images/screenshots/ru/tenant-members.png)

*Тенант: участники и создание пользователей внутри тенанта.*

### Группы тенанта

![DataSafeS3 — Группы тенанта](../images/screenshots/ru/tenant-groups.png)

*Тенант: группы и привязка к внешним группам (IdP).*

### Активность

![DataSafeS3 — Активность](../images/screenshots/ru/activity.png)

*Журнал активности: входы, действия администраторов, события доставки (фильтры и экспорт CSV/JSON).*

### Вебхуки

![DataSafeS3 — Вебхуки](../images/screenshots/ru/webhooks.png)

*Вебхуки: подписки на события объектов (подключён просмотрщик доставок).*

### Профиль и MFA

![DataSafeS3 — Профиль и MFA](../images/screenshots/ru/profile.png)

*Профиль: двухфакторная аутентификация (TOTP), Passkeys (WebAuthn) и смена пароля.*

## Настройки и интеграции

### Настройки

![DataSafeS3 — Настройки](../images/screenshots/ru/settings.png)

*Настройки администратора: мягкое удаление, LDAP/AD и OIDC/SSO.*

### LDAP и OIDC

![DataSafeS3 — LDAP и OIDC](../images/screenshots/ru/settings-ldap.png)

*Подключение LDAP и OIDC (Keycloak): адреса, учётная запись привязки (пароль скрыт), проверка подключения и синхронизация пользователей.*

### Внешнее логирование

![DataSafeS3 — Внешнее логирование](../images/screenshots/ru/settings-sinks.png)

*Внешнее логирование: приёмники журнала (Webhook подключён к демо-просмотрщику).*

### Настройки бакетов

![DataSafeS3 — Настройки бакетов](../images/screenshots/ru/settings-buckets.png)

*Настройки бакетов: массовое применение параметров к нескольким бакетам.*

### Безопасность

![DataSafeS3 — Безопасность](../images/screenshots/ru/settings-security.png)

*Безопасность: публичный доступ, предподписанные ссылки, сроки действия и MFA.*

### Доставка событий

![DataSafeS3 — Доставка событий](../images/screenshots/ru/settings-events.png)

*Доставка событий: Kafka, NATS JetStream и вебхуки — очередь и число доставленных событий.*

### Vault (KMS)

![DataSafeS3 — Vault (KMS)](../images/screenshots/ru/settings-kms.png)

*Vault (KMS): подключение к Vault Transit для шифрования; токен не показывается.*

### Миграция из MinIO

![DataSafeS3 — Миграция из MinIO](../images/screenshots/ru/settings-migrate.png)

*Миграция из MinIO: мастер формирует команду для утилиты миграции.*

### Миграция: параметры

![DataSafeS3 — Миграция: параметры](../images/screenshots/ru/settings-migrate-filled.png)

*Миграция из MinIO: параметры источника и получателя заполнены, команда готова.*

### Миграция: IAM

![DataSafeS3 — Миграция: IAM](../images/screenshots/ru/settings-migrate-iam.png)

*Миграция из MinIO: перенос IAM (пользователи, группы, политики).*

## Шлюз, репликация и кластер

### Шлюз

![DataSafeS3 — Шлюз](../images/screenshots/ru/gateway.png)

*Шлюз: подключения к внешнему S3-хранилищу (демо-площадка Site B) с проверкой соединения и форма добавления подключения.*

### Правила репликации

![DataSafeS3 — Правила репликации](../images/screenshots/ru/gateway-rules.png)

*Шлюз: правила репликации и ручной запуск синхронизации.*

### Задачи синхронизации

![DataSafeS3 — Задачи синхронизации](../images/screenshots/ru/gateway-jobs.png)

*Шлюз: задачи синхронизации и их результат.*

### Состояние шлюза

![DataSafeS3 — Состояние шлюза](../images/screenshots/ru/gateway-status.png)

*Шлюз: состояние — подключения 2/2 OK, одно правило, очередь репликации пуста, реплицировано 4 задачи.*

### Кластеры

![DataSafeS3 — Кластеры](../images/screenshots/ru/cluster.png)

*Кластеры: локальный узел и доверенные кластеры.*

### HA: статус

![DataSafeS3 — HA: статус](../images/screenshots/common/ha-status.png)

*HA из двух узлов: ответ /cluster/status — режим включён, выбран лидер (вывод терминала).*

## Мониторинг и внешние сервисы

### Grafana: обзор

![DataSafeS3 — Grafana: обзор](../images/screenshots/common/monitoring.png)

*Grafana, дашборд обзора: запросы, объём, число объектов, задержки и ошибки HTTP (данные из Prometheus).*

### Grafana: бакеты

![DataSafeS3 — Grafana: бакеты](../images/screenshots/common/grafana-buckets.png)

*Grafana, дашборд бакетов: объём и число объектов по бакетам.*

### Prometheus

![DataSafeS3 — Prometheus](../images/screenshots/common/prometheus-targets.png)

*Prometheus: цель сбора метрик storage-server в состоянии UP.*

### Kafka UI: темы

![DataSafeS3 — Kafka UI: темы](../images/screenshots/common/kafka-ui-topics.png)

*Kafka UI: тема datasafe.events с событиями хранилища.*

### Kafka UI: сообщения

![DataSafeS3 — Kafka UI: сообщения](../images/screenshots/common/kafka-ui-messages.png)

*Kafka UI: сообщения темы datasafe.events — события создания и удаления объектов.*

### Просмотрщик вебхуков

![DataSafeS3 — Просмотрщик вебхуков](../images/screenshots/common/webhook-viewer.png)

*Просмотрщик вебхуков: полученные доставки событий (демо-приёмник на localhost).*

### NATS JetStream

![DataSafeS3 — NATS JetStream](../images/screenshots/common/nats.png)

*NATS JetStream: поток DATASAFE_EVENTS с сообщениями событий (вывод терминала).*

### Vault Transit

![DataSafeS3 — Vault Transit](../images/screenshots/common/vault.png)

*Vault Transit: ключ шифрования бакета и статус Vault (вывод терминала, токен скрыт).*

### MinIO-источник

![DataSafeS3 — MinIO-источник](../images/screenshots/common/minio-src.png)

*MinIO как источник миграции: список объектов тестового бакета (вывод терминала).*

## Вход через LDAP и OIDC (Keycloak)

### Keycloak: вход

![DataSafeS3 — Keycloak: вход](../images/screenshots/common/keycloak-login.png)

*Keycloak: страница входа realm datasafe при входе в консоль через OIDC.*

### Профиль (OIDC)

![DataSafeS3 — Профиль (OIDC)](../images/screenshots/ru/sso-profile.png)

*Профиль пользователя, вошедшего через OIDC (Keycloak).*

### Профиль (LDAP)

![DataSafeS3 — Профиль (LDAP)](../images/screenshots/ru/ldap-profile.png)

*Профиль пользователя, вошедшего через LDAP.*

### Файлы (LDAP)

![DataSafeS3 — Файлы (LDAP)](../images/screenshots/ru/ldap-buckets.png)

*Личное хранилище пользователя LDAP: вошедший пользователь видит только свой бакет files.*

---

Все файлы лежат в [images/screenshots/](../images/screenshots/); список и подписи — в [manifest.json](../images/screenshots/manifest.json). Описание каталога: [images/README.md](../images/README.md).
