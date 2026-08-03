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
./install.sh --dry-run --yes --profiles core,postgres,data
```

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
| 6 | `identity` | off | LDAP + Keycloak lab (`scripts/start-*-test`) |

SSO в консоли — через **OIDC** (Keycloak / другой IdP в Settings), без edge oauth2-proxy.

Тег образа (по умолчанию `v1.2.0`) задаёт `DATASAFE_*_IMAGE`, если не выбран пункт **5**.

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
