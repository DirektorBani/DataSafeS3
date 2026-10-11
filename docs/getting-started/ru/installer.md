**[English](../en/installer.md)** | Русский

# Интерактивный установщик

Быстрый путь к рабочему стеку DataSafeS3: ОС → пререквизиты → меню с пунктами → подтверждение → `docker compose up` → health check.

## Запуск

```powershell
# Windows (PowerShell)
.\install.ps1
```

```bash
# Linux / macOS / WSL / Git Bash
chmod +x install.sh
./install.sh
```

```cmd
install.cmd
```

Без вопросов (CI / скрипты):

```powershell
.\install.ps1 -Yes -Profiles core,postgres,monitoring,data
.\install.ps1 -DryRun -Yes -Profiles core,postgres,data,binary
```

```bash
./install.sh --yes --profiles core,postgres,monitoring,data
./install.sh --yes --profiles core
./install.sh --dry-run --yes --profiles core,postgres,data
```

Один профиль `core` — production-путь одного узла: storage-server и Caddy, метаданные Bolt, dev mode выключен, без Postgres и без мониторинга. `--yes` ничего не спрашивает. Если `http://127.0.0.1:8080/healthz` не готов, `--yes` завершается с ошибкой.

```powershell
.\install.ps1 -Yes -Profiles core -DataRoot "$env:USERPROFILE\datasafe-data"
```

`-Yes` ждёт ответ `http://127.0.0.1:8080/healthz` и завершается с кодом 0. Docker Compose пишет прогресс в stderr. В Windows PowerShell 5.1 этот поток — текст, а не ошибка установки, в том числе если вывод скрипта перенаправлен (`*> log`).

Job CI, который проходит этот путь и бюджет ресурсов: [гейт одного узла](../../operations-guide/ru/single-node-gate.md).

В меню:

- `5` — переключить один пункт  
- `1,2,3,4` или `1 5 6 7` — **задать** набор (включены только перечисленные; core всегда on)  
- `R` — recommended (1–4)  
- `A` — все пункты  
- `C` — подтвердить · `Q` — выход

## Режим Cluster (Wave 1)

В начале установщик спрашивает:

1. **Single-node** — текущий локальный Docker Compose (меню 1–6 ниже).  
2. **Cluster** — несколько Linux-VM (≥3 узла).

Wave 1 собирает inventory и режим SSH; Wave 2 **DryRun** рендерит планы Patroni/etcd, HAProxy/keepalived и NFSv4 C1 (живой remote Apply только с явным `-Apply`):

| Выбор | Смысл |
|--------|---------|
| **P** (по умолчанию) | Пароль root один раз на Apply; Wave 1 **не** пишет пароли на диск |
| **K** | Уже есть SSH-ключи (`BatchMode` в live preflight) |
| Layout | **R** = 4+2 (default) · **2** = 2+1 lab |
| VIP | **V** = floating IP в той же подсети · **D** = DNS / внешний LB |

Inventory: `%USERPROFILE%\.datasafe-cluster\inventory-wave1.json` (без секретов).  
Сгенерированные конфиги: `%USERPROFILE%\.datasafe-cluster\generated\` (в DryRun секреты редactятся).  
Безопасность: [`scripts/cluster/SECURITY.md`](../../../scripts/cluster/SECURITY.md).  
Проверки: `scripts/tests/cluster-installer-w1.ps1` · `w2.ps1` (Windows) · `scripts/tests/cluster-installer-w1.sh` · `w2.sh` (bash, без pwsh)  
DryRun: `.\install.ps1 -Cluster -DryRun -Yes` · `./install.sh --cluster --dry-run --yes`

Bash Wave 2 DryRun: `scripts/cluster/cluster_render_w2.sh` / `cluster_apply_w2.sh`.  
Живой Apply (явно): `./scripts/cluster/cluster_apply_w2.sh --apply --inventory ~/.datasafe-cluster/inventory-wave1.json --identity ~/.ssh/id_ed25519`  
Lab: `bash scripts/cluster/lab/up.sh && bash scripts/cluster/lab/run-apply.sh && bash scripts/cluster/lab/run-drills.sh --lab`  
storage-server ставится на leader Apply-скриптом или offline lab compose. Drills в lab; timed Patroni promote / keepalived VIP на bare metal — отдельный follow-up, не GA multi-AZ.

## Меню → что ставится (single-node)

| # | Id | Default | Эффект |
|---|-----|---------|--------|
| 1 | `core` | on (обязательно) | `docker-compose.yml` — storage-server + Caddy |
| 2 | `postgres` | on | `--profile postgres`, `STORAGE_METADATA_BACKEND=postgres` |
| 3 | `monitoring` | on | Prometheus + Grafana (если off — не стартуют) |
| 4 | `data` | on | `docker-compose.local-data.yml` + каталог `DATASAFE_DATA_ROOT` |
| 5 | `binary` | off | `deploy/compose/docker-compose.local-binary.yml` + сборка бинарника и console |
| 6 | `identity` | off | LDAP + Keycloak lab (`scripts/start-*-test`). Включает **dev mode**, чтобы HTTP IdP лаборатории был доступен. |
| 7 | `dev` | off | `STORAGE_DEV=true` (HTTP/private outbound, без предупреждений о дефолтных секретах). Также `-Dev` / `--dev`. |
| 8 | `kafka` | off | `deploy/compose/docker-compose.kafka.yml` — один брокер Apache Kafka KRaft (`apache/kafka:3.9.2`). Задаёт `STORAGE_KAFKA_BROKERS=kafka:19092`. Не входит в `--yes` без `--profiles`. |

SSO в консоли — через **OIDC** (Keycloak / другой IdP в Settings), без edge oauth2-proxy.

Тег образа (по умолчанию `v1.4.1`) задаёт `DATASAFE_*_IMAGE`, если не выбран пункт **5**.

## Dev mode и metrics (v1.4.1)

Установка по умолчанию ставит `STORAGE_DEV=false`. Dev mode — явный opt-in (`-Dev`, `--dev` или профиль `dev`). Профиль **identity** тоже ставит `STORAGE_DEV=true`, потому что issuer Keycloak в лаборатории — обычный HTTP. Не используйте этот профиль в production.

Установщик генерирует `STORAGE_JWT_SECRET`, `STORAGE_SECRET_KEY` и `STORAGE_METRICS_TOKEN`, если их нет или они всё ещё равны опубликованным дефолтам. `STORAGE_ADMIN_PASSWORD` не меняется (`admin` / `admin` до мастера настройки). При включённом monitoring Prometheus скрейпит `/metrics` с этим bearer-токеном, и дашборды Grafana продолжают работать. Повторный запуск сохраняет уже заданные недефолтные секреты.

Первый запуск без изменений: открыть консоль, сменить пароль admin, пройти setup wizard (внешний S3 необязателен).

## Пререквизиты

Проверяются Docker Engine и `docker compose`. Если нет — **предложение** установить (`winget` / `brew` / пакеты дистрибутива), без тихой установки. С `--yes` / `-Yes` отсутствие Docker — ошибка.

## После установки

| Сервис | URL |
|--------|-----|
| Консоль | http://localhost:8080 (`admin` / `admin`) |
| S3 API | http://localhost:9000 |
| Grafana | http://localhost:3000 (если monitoring) |

Дальше: сменить пароль admin → setup wizard → [onboarding](onboarding.md).

Overlays: [`deploy/compose/README.md`](../../deploy/compose/README.md).
