**[English](../en/single-node-gate.md)** | Русский

# Регрессионный гейт одного узла

Job CI `single-node-gate` проверяет, что pull request по-прежнему запускается одним production-контейнером: встроенный Bolt, `STORAGE_DEV=false`, без процессов Postgres, NATS и Kafka, и пустой outbox уведомлений, когда получатели не настроены.

Вошло в **v1.5.0**. Job не публикует образ и не пушит тег. С 1.6.0 бюджет ресурсов сравнивает эту сборку с опубликованным образом **v1.5.0**: Bolt с Bolt, Postgres с Postgres. Перетегирование установщика использует пин **v1.6.0** и выполняется после этого измерения.

## Что он гарантирует

На каждом pull request в `main` и на push в `main`:

1. Job собирает `storage-server` из `deploy/docker/Dockerfile` и запускает **только этот контейнер**. Окружение совпадает с профилем установщика `core`: метаданные Bolt, dev mode выключен, bearer для метрик, JWT и S3-секрет не дефолтные, пароль admin по-прежнему `admin`, а `STORAGE_POSTGRES_HOST=postgres` задан так же, как в Compose. Нет `STORAGE_NATS_URL` и нет `STORAGE_KAFKA_BROKERS`. Postgres, Prometheus и Grafana не стартуют.
2. Smoke через AWS CLI (path-style): `/healthz`, вход admin, смена пароля первого запуска и `POST /api/v1/setup/complete`, S3 create-bucket / put / get / list, в Activity есть хотя бы одна запись, создание и список inventory, скачивание CSV inventory, экспорт Activity в CSV и JSON. Эти файлы пакуются в zip так же, как `scripts/collect-evidence-pack.ps1`.
3. Без sink'ов `datasafe_events_outbox_pending` равен 0, а счётчики delivered, dropped и dead отсутствуют или равны нулю. В логах сервера не должно быть дозвона к брокеру (`nats://`, `kafka`, `amqp://`, `dial tcp`, `lookup postgres`, `no such host`, `connection refused`). `/proc/net/tcp` и `tcp6` внутри контейнера не должны показывать исходящий сокет, кроме loopback (healthcheck образа) и соединений, у которых локальный порт 9000 (входящий smoke).
4. Тот же smoke повторяется после `./install.sh --yes --profiles core`. Этот путь поднимает только `storage-server` и Caddy. Вызовы Admin идут через Caddy на порту 8080. S3 остаётся на порту 9000.
5. Idle RSS и время от старта до healthy для Bolt сравниваются с `ghcr.io/direktorbani/datasafe-storage-server:v1.5.0` на том же runner, оба на Bolt. Job падает, если медиана старта PR больше медианы baseline больше чем на 1 секунду, или медиана idle RSS PR больше медианы baseline больше чем на 10%. Таблица попадает в summary шага GitHub. Лимит +10% не повышается.
6. Postgres — отдельное измерение тех же двух образов, оба с `STORAGE_METADATA_BACKEND=postgres`, рядом с одним контейнером Postgres 16. Прогоны чередуются. Каждый прогон начинает с новой базы `datasafe`, чтобы образ v1.5.0 не открывал базу, которую эта сборка уже мигрировала. Те же лимиты +10% RSS и +1 с на старт. Числа Bolt не являются baseline для Postgres.

`install.ps1 -Yes -Profiles core` — то же на Windows. CI на `windows-latest` только разбирает скрипт. Docker там не запускается.

## Как измеряется бюджет

Пять прогонов каждого образа, по очереди (baseline, PR, baseline, PR, …), чтобы медленный участок runner задел оба. Старт — от создания контейнера до ответа `GET /healthz`. После 5 секунд прогрева job три раза читает `VmRSS` PID 1 с паузой в секунду и берёт медиану этих трёх. Лимиты считаются по медианам прогонов. Рядом печатаются минимум и максимум.

Runner, который медленный на большей части проб PR, может завалить сборку, которая на другой машине проходит. Поэтому в summary есть min и max. Сами лимиты job не раздвигает.

## Локальный запуск (Linux)

Нужны Docker, curl и python3. Для S3 используется AWS CLI; скрипт ставит его через `apt-get`, если его нет и доступен `sudo`.

```bash
python3 scripts/single-node-gate.py self-test

docker build -t datasafe-storage-server:gate -f deploy/docker/Dockerfile .
docker pull ghcr.io/direktorbani/datasafe-storage-server:v1.5.0

BASE_IMAGE=ghcr.io/direktorbani/datasafe-storage-server:v1.5.0 \
PR_IMAGE=datasafe-storage-server:gate \
bash scripts/single-node-gate.sh all
```

`all` печатает таблицу бюджета и затем гоняет smoke образа PR на порту 9000. `measure` и `smoke` — две половины. `GATE_RUNS` (по умолчанию 5) и `GATE_WARMUP_SEC` (по умолчанию 5) меняют выборку.

Профиль core через установщик, на только что собранном образе (у Compose `pull_policy: if_not_present`, поэтому побеждает локальный тег; в реестр ничего не пушится):

```bash
docker build -t ghcr.io/direktorbani/datasafe-console:v1.6.0 -f deploy/docker/Dockerfile.console .
docker tag datasafe-storage-server:gate ghcr.io/direktorbani/datasafe-storage-server:v1.6.0
./install.sh --yes --profiles core --project datasafe-gate --data-root "${TMPDIR:-/tmp}/datasafe-gate"
bash scripts/single-node-gate.sh smoke-installed
docker compose -p datasafe-gate -f docker-compose.yml down -v
```

CI дополнительно распаковывает `git archive HEAD` (без `web/console/dist`) и запускает тот же профиль core на `PUBLIC_HTTP_PORT=18080`. `GET /` должен быть 200 и содержать `id="root"`.

Не указывайте `measure` на этот перетегированный образ. Скрипт отказывается сравнивать два образа с одним id. Сначала снова сделайте `docker pull` для `v1.5.0`. `measure-postgres` — та же пара образов с Postgres и тоже отказывается от перетегированного baseline.

Windows, из корня репозитория (Docker Desktop запущен):

```powershell
.\install.ps1 -Yes -Profiles core -DataRoot "$env:USERPROFILE\datasafe-data"
```

`-Yes` пропускает меню, вопрос про каталог данных и подтверждение. Профиль `core` оставляет выключенными Postgres, мониторинг, тома данных на хосте, identity lab и dev mode. Если граница консоли не стала healthy (`/healthz` и `GET /` с `id="root"`), `-Yes` завершается с ошибкой.

## SSE-объект 1.5.0

`bash scripts/single-node-gate.sh legacy-sse` — отдельный шаг. Он не входит в выборку idle RSS и старта и не меняет лимиты +10% и +1 с. Опубликованный образ v1.5.0 записывает один объект с `STORAGE_SSE_MASTER_KEY` chunked PUT (PUT с Content-Length на 1.5.x упирается в ошибку размера и ничего не сохраняет). Этот образ читает объект тем же ключом при `STORAGE_SSE_LEGACY_FORMAT0=true`. Переменные `STORAGE_KMS_*` не задаются.

## Честно

- Пустота outbox — это ряд Prometheus, а не число ключей Bolt. Gauge pending регистрируется при старте процесса, поэтому тихий сервер всё равно отдаёт 0.
- Проверка TCP не видит UDP, включая DNS. Ошибка резолва до TCP-соединения должна попасть в шаблоны логов выше.
- Входящие клиенты (smoke, Caddy, healthcheck) разрешены. Любой другой сокет не на loopback валит job, включая порт брокера.
- Zip evidence собирает `scripts/single-node-gate.sh` из Admin API. Linux CI не запускает `collect-evidence-pack.ps1`.
- Шаг установщика локально перетегирует образ PR в `ghcr.io/direktorbani/datasafe-storage-server:v1.6.0`, чтобы `./install.sh` (тег по умолчанию `v1.6.0`) поднял биты PR. Job этот тег не пушит. Перетегирование идёт после измерения бюджета. Обычная установка без него тянет опубликованный образ v1.6.0, когда тег уже есть. Baseline бюджета остаётся опубликованный образ v1.5.0, отдельно для Bolt и для Postgres.
- Вход в `ghcr.io` перед pull baseline необязателен. Fork pull request его пропускает, а неудачный login не валит job. Образ `v1.5.0` публичный.
- Windows job в CI только проверяет, что `install.ps1` разбирается и документирует `-Yes -Profiles core`.
- В старт входит время создания контейнера Docker. Процедура одна и та же для обоих образов; лимит применяется к разнице медиан.
