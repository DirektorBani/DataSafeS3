# Инструкция по установке DataSafeS3

**Релиз:** v1.7.0  
**Лицензия:** Apache-2.0  
Интерфейс консоли доступен на русском и английском.

Эта инструкция ставит сервер. Работа в консоли описана в [руководстве пользователя](user-guide.md).

---

## 1. Что получится после установки

Один узел: процесс `datasafe-storage-server`, метаданные Bolt в файле рядом с объектами, S3 API на порту **9000**, веб-консоль на порту **8080** (через Caddy или сервис консоли чарта). Отдельные PostgreSQL, Kafka и Vault не требуются и в таком режиме не запускаются.

Дальше, если нужно:

| Режим | Когда | Что добавляется |
|-------|--------|-----------------|
| Один узел | Обычная установка | Только Bolt и том с данными |
| Несколько узлов (HA) | Общие метаданные и копия байтов на соседей | PostgreSQL 16, `STORAGE_HA_ENABLED`. Это не автоматический multi-AZ |
| Кластер установщика | Несколько Linux-ВМ, лабораторный контур | Сценарий `install.ps1 -Cluster` / `install.sh --cluster`. Живой apply только с явным флагом. Это не готовый multi-AZ |

События (вебхук, NATS, Kafka) и Vault Transit включаются отдельно и в простой установке молчат.

---

## 2. Требования

### Общие

| Компонент | Минимум по чарту и пакетам |
|-----------|----------------------------|
| Процесс сервера | Запрос чарта: 100m CPU и 256 МиБ ОЗУ. Предел чарта: 1 CPU и 1 ГиБ. На данные закладывайте больше |
| Консоль | Запрос чарта: 50m CPU и 64 МиБ. Предел: 500m CPU и 256 МиБ |
| Диск | Том данных. В чарте PVC по умолчанию 20 ГиБ. Пакет кладёт данные в `/var/lib/datasafes3`. Место растёт вместе с объектами |
| Сеть | Вход на **9000** (S3 и `/healthz`, `/metrics`) и **8080** (консоль). Наружу 9000 не публикуйте, если TLS уже завершает прокси |
| Архитектура образов и пакетов | `linux/amd64` и `linux/arm64` |

Часы, телефон и юридический адрес площадки в требования к ПО не входят: сервер их не проверяет.

### По способу установки

| Способ | Что нужно заранее |
|--------|-------------------|
| Docker Compose | Docker Engine и Compose v2. На Windows — Docker Desktop или другой движок, который выполняет `docker compose` |
| Helm | Kubernetes **1.25** и новее (`kubeVersion` чарта `datasafes3`), Helm 3, StorageClass для PVC |
| .deb | Debian или Ubuntu, amd64 или arm64, systemd |
| .rpm | Fedora, RHEL, Rocky или Alma, x86_64 или aarch64, systemd |
| Windows | `install.ps1` или `install.cmd` в корне репозитория. Отдельного MSI в репозитории нет. Скрипт собирает стек через Docker |

PostgreSQL **16** нужен только если метаданные вынесены из Bolt (`STORAGE_METADATA_BACKEND=postgres`), в том числе в режиме нескольких узлов. Образ в чарте: `postgres:16-alpine`.

---

## 3. Образы и пакеты

Все артефакты релиза лежат на GitVerse: [репозиторий](https://gitverse.ru/DirektorBani/DataSafeS3), [пакеты](https://gitverse.ru/DirektorBani/DataSafeS3/packages). Реестр публичный, для скачивания вход не нужен.

Образы релиза (реестр `gitverse.ru`, владелец `direktorbani` строчными буквами, `linux/amd64` и `linux/arm64`):

- `gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0`
- `gitverse.ru/direktorbani/datasafe-console:v1.7.0`

Тег `latest` указывает на тот же релиз.

```bash
docker pull gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0
docker pull gitverse.ru/direktorbani/datasafe-console:v1.7.0
```

Пакеты Linux (`datasafes3`, `amd64` и `arm64`) ставятся из репозиториев GitVerse:

| Что | Адрес репозитория | Система |
|-----|-------------------|---------|
| DEB | `https://gitverse.ru/api/packages/DirektorBani/debian` (дистрибутив `stable`, компонент `main`) | Debian, Ubuntu |
| RPM | `https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo` | Fedora, RHEL, Rocky, Alma |
| Helm | `https://gitverse.ru/api/packages/DirektorBani/helm` | Kubernetes |

Команды подключения — в разделах 5 и 6. Зеркало на GitHub (`ghcr.io`, GitHub Release с `SHA256SUMS` и cosign-bundle) остаётся запасным вариантом: проверка подписи описана в [политике безопасности](security-policy.md).

---

## 4. Docker Compose

### 4.1. Один узел, production-файл

Каталог `deploy/compose/production`: том, healthcheck, `STORAGE_DEV` жёстко `false`, S3 только на `127.0.0.1`, Caddy спереди. `/metrics` Caddy не проксирует.

```bash
cp deploy/compose/production/.env.example deploy/compose/production/.env
# замените секреты в .env
docker compose --env-file deploy/compose/production/.env \
  -f deploy/compose/production/docker-compose.yml up -d
```

Консоль: `http://127.0.0.1:8080`. S3: `http://127.0.0.1:9000`.

TLS: положите `fullchain.pem` и `privkey.pem` в `deploy/compose/production/certs` и добавьте `--profile tls` (порт **8443**).

Подробнее: [deploy-production.md](deploy-production.md).

### 4.2. Интерактивный установщик

Из корня репозитория:

```powershell
# Windows
.\install.ps1
```

```bat
install.cmd
```

```bash
chmod +x install.sh && ./install.sh
```

Меню: Core, Postgres, Monitoring, данные на хосте, необязательные сборка, лаборатория идентичности и Kafka. Kafka в набор по умолчанию не входит. Один профиль `core` — сервер и Caddy, Bolt, без Postgres и без мониторинга.

Без вопросов:

```bash
./install.sh --yes --profiles core
```

Установщик ждёт `http://127.0.0.1:8080/healthz` и ответ `GET /` с маркером консоли `id="root"`.

### 4.3. Ручной стек разработчика

```bash
cp .env.example .env
docker compose -p datasafe --profile postgres -f docker-compose.yml -f docker-compose.local-data.yml up -d
```

`STORAGE_DEV` в этом стеке по умолчанию выключен. Образ консоли — `gitverse.ru/direktorbani/datasafe-console:v1.7.0`. Для Vite при правке интерфейса добавьте `-f deploy/compose/docker-compose.dev.yml --profile dev`.

Оверлеи лабораторий (HA, erasure, Vault, Kafka) перечислены в [deploy-compose.md](deploy-compose.md). Порты лабораторий не занимают `:9000` и `:9001` одновременно с основным стендом: erasure — консоль **8084**, S3 **9004**, Postgres **5444**; площадка A — **8086** / **9006** / **5446**; HA — **8082** / **9002** / **5442**. Старый скопированный `.env.erasure` или `.env.site-a` хранит прежние порты, пока его не пересоздадут из примера.

---

## 5. Helm

Текущий чарт релиза — `datasafes3`. По умолчанию StatefulSet, PVC и Bolt. Секреты задаются явно: с пустыми чарт ставить нельзя.

Из репозитория чартов GitVerse:

```bash
helm repo add datasafes3 https://gitverse.ru/api/packages/DirektorBani/helm
helm repo update
helm install ds datasafes3/datasafes3 --version 1.7.0 \
  --set storageServer.image.repository=gitverse.ru/direktorbani/datasafe-storage-server \
  --set console.image.repository=gitverse.ru/direktorbani/datasafe-console \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Из клона репозитория (в нём образы по умолчанию уже указывают на GitVerse):

```bash
helm install ds deploy/helm/datasafes3 \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Проброс портов (имя релиза `ds`, неймспейс `default`):

```bash
kubectl -n default port-forward svc/ds-datasafes3-s3 9000:9000
kubectl -n default port-forward svc/ds-datasafes3-console 8080:8080
```

`values-production.yaml` не отрисуется без `auth.existingSecret`. `values-ha.yaml` включает Postgres и `STORAGE_HA_ENABLED`. Kafka и NATS туда не ставятся: их задают полями `events.kafkaBrokers` и `events.natsURL`, и только тогда процесс к ним подключается.

Прежний чарт `deploy/helm/datasafe` повторяет более ранний стек с Caddy, Prometheus и Grafana. Для новой установки берите `datasafes3`. Оба описаны в [kubernetes.md](kubernetes.md).

Под чарта: UID 65532, корневая файловая система только для чтения, данные на томе. Пробы ходят в `GET /healthz`. PodDisruptionBudget включён (`maxUnavailable: 1`). NetworkPolicy и ServiceMonitor выключены, пока их не включат.

---

## 6. Пакеты Linux

Пакет ставит бинарники, unit и каталоги и **не запускает** службу: в файле окружения исторические секреты, а `STORAGE_STRICT_SECRETS=true` не даст процессу стартовать, пока их не заменят.

Debian и Ubuntu:

```bash
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://gitverse.ru/api/packages/DirektorBani/debian/repository.key \
  -o /etc/apt/keyrings/gitverse-DirektorBani.asc
echo "deb [signed-by=/etc/apt/keyrings/gitverse-DirektorBani.asc] https://gitverse.ru/api/packages/DirektorBani/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/gitverse-DirektorBani.list
sudo apt update
sudo apt install datasafes3
```

Fedora, RHEL, Rocky и Alma:

```bash
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo
sudo dnf install datasafes3 --nogpgcheck
```

RPM-пакеты не подписаны GPG, поэтому `dnf` просит `--nogpgcheck`. Метаданные репозитория подписаны (`repomd.xml.asc`), загрузка идёт по HTTPS.

Дальше, для обоих семейств:

```bash
sudoedit /etc/datasafes3/datasafes3.env
sudo systemctl enable --now datasafes3
curl -sS http://127.0.0.1:9000/healthz
```

Замените `STORAGE_ADMIN_PASSWORD`, `STORAGE_SECRET_KEY` и `STORAGE_JWT_SECRET`. Если порт 9000 доступен не только с localhost, задайте `STORAGE_METRICS_TOKEN`.

Состав пакета, обновление и удаление: [linux-packages.md](linux-packages.md). Удаление пакета не удаляет `/var/lib/datasafes3`.

Консоль в пакет не вшита как отдельный сайт. JSON API и S3 слушает `datasafe-storage-server` на порту 9000. Чтобы получить ту же веб-консоль, что в Compose, поставьте образ `datasafe-console` за обратным прокси или используйте Compose / Helm. Swagger процесса: `http://127.0.0.1:9000/api/v1/docs`.

Подробности про реестры GitVerse: [gitverse-mirror.md](gitverse-mirror.md).

---

## 7. Windows

Отдельного установщика MSI или EXE в репозитории нет.

1. Установите Docker.
2. Клонируйте репозиторий.
3. Запустите `install.ps1` или `install.cmd`, как в разделе 4.2.

Скрипт не копирует данные на диск D: сам. Каталог задаётся `DATASAFE_DATA_ROOT` (в примере `.env` для локальной разработки Windows указан `D:/datasafe-data`). На Linux и macOS задайте свой путь, например `/var/lib/datasafe`.

Кластерный режим установщика спрашивает инвентарь SSH и рисует планы Patroni, HAProxy и keepalived. Запись на машины выполняется только с явным `-Apply` / `--apply`. Это лабораторный контур, не поставка «включи и забудь» для нескольких зон.

---

## 8. Первый запуск

1. Откройте консоль (`http://127.0.0.1:8080` для Compose).
2. Войдите `admin` / `admin`, если пароль ещё не меняли.
3. Смените пароль администратора.
4. Завершите мастер настройки. Внешний S3 можно пропустить.
5. Создайте бакет в консоли или через S3 API.
6. Для боевого контура замените ключ S3, секрет, пароль администратора и `STORAGE_JWT_SECRET`. Включите `STORAGE_STRICT_SECRETS=true`, если способ установки этого ещё не сделал. Задайте `STORAGE_METRICS_TOKEN`.

Чеклист перед тем, как узел станет основной копией: [production-checklist.md](production-checklist.md).

Проверка:

```bash
curl -sS http://127.0.0.1:9000/healthz
```

В JSON есть `status` и `version`.

---

## 9. Что не ставить «на всякий случай»

| Компонент | Поведение, если его не настраивали |
|-----------|-------------------------------------|
| Kafka | Пустой `STORAGE_KAFKA_BROKERS`: нет клиента, нет дочернего процесса моста, нет соединения. Профиль установщика `kafka` не входит в набор по умолчанию |
| NATS | Пустой `STORAGE_NATS_URL`: приёмник не регистрируется |
| Vault Agent | Не входит в production Compose. Секреты читаются из env-файла или Secret Kubernetes |
| Vault Transit | Клиент создаётся только при `STORAGE_KMS_PROVIDER=vault-transit` и непустом адресе |
| PostgreSQL | Не нужен одному узлу на Bolt |

Вебхук включается, когда в настройках есть хотя бы один включённый приёмник. До этого события объектов в очередь исходящих для пустого списка приёмников не копятся.

---

## 10. Куда дальше

- [Руководство пользователя](user-guide.md)
- [Жизненный цикл и обновления](lifecycle.md)
- [Порядок поддержки](support.md)
