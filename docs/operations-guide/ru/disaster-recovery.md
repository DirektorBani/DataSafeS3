**[English](../en/disaster-recovery.md)** | Русский

# Аварийное восстановление

## RPO / RTO (HA v2)

| Стратегия | RPO (лаб.) | RTO (лаб.) | Сложность |
|-----------|------------|------------|-----------|
| Ежедневный tarball | 24ч | Часы | Низкая |
| Postgres streaming + erasure | Секунды (lag метаданных) | ≤ 5 мин (скрипт failover) | Средняя |
| Site replication (peer DataSafeS3) | 30–60 с async | Минуты | Средняя |
| Gateway → external S3 | Минуты | Часы | Средняя |
| Legacy shared-FS standby | = диск primary | N/A (только чтение) | **deprecated** |

**Автоматический failover** всего стека не предполагается без Patroni (или аналога). CE поставляет **скрипты**, не постоянный контроллер failover.

## Скриптовый promote

Это команда, которую запускает оператор. Это не автоматический failover между зонами доступности.

```bash
bash scripts/ha/promote.sh --dry-run --primary <старый> --standby <реплика>
bash scripts/ha/promote.sh --execute --primary <старый> --standby <реплика>
```

Windows: `scripts\ha\promote.ps1 -DryRun` или `-Execute -PrimaryContainer <старый> -StandbyContainer <реплика>`. Для Patroni — `scripts/cluster/promote.sh` (те же коды выхода). Повторный запуск безопасен: уже повышенный standby и уже ограждённый старый primary не меняются. Имена контейнеров каждый раз одни и те же. `PRIMARY` — узел, который ограждается, даже если он уже не принимает запись.

### Коды выхода

| Код | Значение |
|-----|----------|
| 0 | Promote выполнен, либо это состояние уже было. Dry-run и план тоже завершаются 0. |
| 1 | Неверные аргументы. |
| 2 | Проверка до promote. Lag выше `--max-lag-bytes` (по умолчанию 1 МиБ), standby не запущен, или со старого primary не видно streaming. |
| 3 | Ограждение не удалось: старый primary всё ещё принял запись. |
| 4 | Не удался `pg_promote` или switchover Patroni. |
| 5 | Проверка после promote: новый primary всё ещё в recovery или отказал в записи. |

### Ограждение

Перед `pg_promote` скрипт ставит на старом primary `default_transaction_read_only=on` через `ALTER SYSTEM` и делает reload. `postgresql.auto.conf` не реплицируется, поэтому повышенный standby остаётся доступен для записи. Сессии клиентов обрываются. Новая сессия не может выполнить `CREATE TABLE`.

Это не отключение питания. Суперпользователь может в сессии выполнить `SET transaction_read_only = off`. storage-server так не делает. Когда продление leader-аренды падает на read-only базе, процесс сразу снимает с себя аренду (`internal/ha`), и `leaderGuard` отвечает на запись `not cluster leader`. Старый TTL не выжидается. Повторно аренду процесс берёт только после запуска уже на новом primary.

Диспетчер событий делает то же самое, как только запись в outbox отвергнута. Транзакция read-only (`SQLSTATE 25006`) или ответ хранилища «не лидер» останавливает цикл и снимает аренду. Этот процесс строку больше не публикует. Строка, которую уже отдали получателю и не успели отметить доставленной, может быть доставлена новым лидером ещё один раз.

Patroni вместо этого понижает старый primary: узел снова входит в recovery и отказывает в записи. Кластерный скрипт `ALTER SYSTEM` не выполняет.

После promote укажите новому storage-server DSN нового Postgres и перезапустите его. Первый promote удаляет `ha_leader_lock` на новом primary, чтобы этот узел мог взять аренду. Повторный запуск живую аренду не трогает и не снимает нового лидера. Диспетчер событий работает только на держателе аренды.

### Лаборатория одной командой

```bash
bash scripts/ha/run-promote-drill.sh
```

Задание CI `ha-promote-drill` запускает эту команду на образе из `deploy/docker/Dockerfile` (артефакт `docker-build`). Поднимаются Postgres primary и replica, узел A, затем узел B только после того как A держит аренду, и webhook. Во время promote идут записи объектов, затем `scripts/collect-dr-evidence.sh` пишет каталог, `summary.md`, `evidence.json` и zip. Порты хоста по умолчанию 19100, 19101, 15442, 15443 и 18080: лаб не занимает `:9000` и `:9001` (стенд Vault). Переопределение: `HA_DRILL_HTTP_A`, `HA_DRILL_HTTP_B`, `HA_DRILL_PG_A`, `HA_DRILL_PG_B`, `HA_DRILL_WEBHOOK`. Публичные порты standby HA-стека по умолчанию **19011** и **19013** (`HA_STANDBY_HTTP_PORT`, `HA_STANDBY2_HTTP_PORT`).

## Восстановление

### Postgres / метаданные

1. `scripts/ha/promote.sh --execute` (или `scripts\ha\promote.ps1 -Execute`, или `scripts\ha\failover-metadata.ps1 -Execute`). Сначала dry-run.
2. Указать `STORAGE_POSTGRES_DSN` на новый primary и перезапустить этот storage-server со своим `STORAGE_NODE_ID`.
3. Проверить `/healthz`: на новом узле `is_leader=true`, старый primary не лидер и отказывает в SQL-записи.

### Erasure objects

1. Восстановить **все** пути из `STORAGE_ERASURE_DATA_PATHS`.
2. Запустить с `STORAGE_OBJECT_BACKEND=erasure`; дождаться `erasure_degraded=false`.

### Переключение на Site B

1. Promote Postgres на Site B (отдельный деплой).
2. DNS/Ingress на Site B.
3. Проверить GET объекта и `GET /api/v1/site-replication/status`.

Ежеквартальный DR drill: восстановление в изолированное окружение, `bash scripts/ha/run-promote-drill.sh` или `scripts\ha\test-ha-cluster.ps1`, затем `summary.md` из пакета.

`scripts/collect-dr-evidence.sh` (и `.ps1`) записывает время promote, lag репликации до и после (или `n/a`, если старый primary ограждён и репликации нет), повторный запуск (был ли он, код выхода и что пропущено), healthz, запись и чтение объекта на новом лидере, состояние outbox, что диспетчер работает на новом лидере, потерю (пакет при этом неуспешен) и версии компонентов. Одна лишняя доставка ключа, который был в полёте, допустима. Вторая лишняя доставка того же ключа делает пакет неуспешным. В `evidence.json` на верхнем уровне есть `verdicts`: по одному `PASS` или `FAIL` на каждую проверку.

### Чем это не является

- Автоматический failover между зонами доступности. Сам по себе лидер здесь не выбирается.
- Перенос байтов объектов вместе с метаданными. Диск каждого узла свой. Чтение после promote — это новый объект, записанный уже на новом лидере.
- Отключение питания (STONITH). Сессия суперпользователя может снять read-only.
- Сборка ограждённого primary обратно в standby. Обратный `pg_basebackup` скрипт не делает.
- SSH-лаборатория в release gate гоняет dry-run promote и пакет по healthz. Тайминг Patroni по-прежнему измеряется остановкой контейнера лидера. Compose-задание — это promote с трафиком.
