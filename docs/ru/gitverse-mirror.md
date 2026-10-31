# GitVerse — основной репозиторий

С 24.10.2026 основной репозиторий DataSafeS3 — [DirektorBani/DataSafeS3 на GitVerse](https://gitverse.ru/DirektorBani/DataSafeS3). Коммиты, запросы на слияние и CI живут там. GitHub — только зеркало для чтения: туда копируются `main`, ветки `release/**` и теги `v*`.

## Зеркало GitVerse → GitHub

Workflow [`.gitverse/workflows/mirror_to_gh.yaml`](../../.gitverse/workflows/mirror_to_gh.yaml) срабатывает на push в `main`, `release/**` и теги `v*`, а также вручную. Скрипт [`scripts/github-mirror.sh`](../../scripts/github-mirror.sh) отправляет на GitHub только эту ссылку.

Это не `git push --mirror`. Ветки, которых нет на GitVerse (ветки агентов Cursor, `refs/pull/*`), на GitHub остаются. Ветки не форсятся: если история разошлась, push отклоняется и job завершается с ошибкой. Теги отправляются с force, чтобы перенесённый на GitVerse релизный тег обновился и на GitHub.

Секреты репозитория GitVerse: `GH_MIRROR_ACTOR` (владелец на GitHub), `GH_MIRROR_REPO` (имя репозитория), `GH_MIRROR_TOKEN` (токен GitHub с правами Contents и Workflows на запись, либо classic со scopes `repo` и `workflow`). Токен в URL не попадает: на запрос пароля отвечает `GIT_ASKPASS`, вывод `git push` проходит через маскирование.

Релиз собирается и публикуется на GitVerse ([build.md](build.md)), от зеркала на GitHub он не зависит. Лимит облачных раннеров GitVerse — 1000 минут в месяц на публичный репозиторий, поэтому зеркалируются только основные ветки и теги.

Прежнее направление GitHub → GitVerse (`.github/workflows/mirror-gitverse.yml`) отключено. Описание ниже относится к нему и сохранено для истории и для публикации пакетов.

---

## Прежняя схема: GitHub → GitVerse (отключена)

Репозиторий [DirektorBani/DataSafeS3](https://gitverse.ru/DirektorBani/DataSafeS3) получает ветки и теги с GitHub и хранит пакеты релиза. Это обычный репозиторий GitVerse, а не импорт в режиме зеркала: у импортированного зеркала CI/CD выключен ([FAQ](https://gitverse.ru/docs/cicd/faq/)).

## Зеркало исходного кода

`.github/workflows/mirror-gitverse.yml` запускается на каждый push любой ветки и любого тега, а также вручную (`workflow_dispatch`). Скрипт [`scripts/gitverse-mirror.sh`](../../scripts/gitverse-mirror.sh) забирает с GitHub все `refs/heads/*` и `refs/tags/*` и отправляет их на `https://gitverse.ru/DirektorBani/DataSafeS3.git`.

Это не `git push --mirror`. Ссылки, которых нет на GitHub, на GitVerse не удаляются: своя ветка, свой тег и ветка, уже удалённая на GitHub, остаются. Обновление только fast-forward. Если история разошлась, эта ссылка не перезаписывается, остальные всё равно уходят, и job завершается с ошибкой.

Форс-пуш включается отдельно. Он перезаписывает разошедшиеся ветки и теги, которые указывают на другой коммит. Чужие ссылки он тоже не удаляет.

- Ручной запуск: input `force` (по умолчанию выключен). Значение `false` не подменяется переменной репозитория.
- На обычном push: переменная репозитория `GITVERSE_MIRROR_FORCE` = `true`. Пока переменная не задана, форс-пуша нет.

Проверка скрипта без сети: `bash scripts/gitverse-mirror-test.sh`. Тот же шаг есть в workflow до push.

### Как передаётся токен

Рабочий способ по документации GitVerse — HTTPS. Логин `DirektorBani`, вместо пароля персональный токен ([клонирование с токеном](https://gitverse.ru/docs/collaborative/repositories/guides/repository-workflow/clone-repository), [работа по HTTPS](https://gitverse.ru/docs/collaborative/authentification/work-over-https)). В URL токена нет: на запрос пароля отвечает `GIT_ASKPASS`. В командах нет `set -x`. Вывод `git push` проходит через редактор, который вырезает токен, приватный ключ и userinfo в URL. Секрет GitHub дополнительно маскируется раннером.

Токен для git должен быть выпущен с чекбоксом «Репозитории» ([управление токенами](https://gitverse.ru/docs/collaborative/authentification/tokens)). В той же статье токен реестра пакетов описан отдельной процедурой. Совмещаются ли чекбокс «Репозитории» и права записи пакетов в одном токене, страница не говорит. Секрет `GITVERSE_TOKEN` уже использует публикация пакетов. Если этот токен только для пакетов, git push получит отказ. Тогда пакетный токен не заменяют, а для зеркала включают SSH (ниже).

SSH — запасной путь, если HTTPS-токен к git push не подходит ([ключи SSH](https://gitverse.ru/docs/collaborative/authentification/ssh-keys)). URL: `git@gitverse.ru:DirektorBani/DataSafeS3.git`. Host key зафиксирован по отпечатку `SHA256:IL4khwaSrT4uHCu9LTUW7dRJyMB5XkOJnFrP0nmdMXU` ([отпечатки](https://gitverse.ru/docs/collaborative/authentification/gitverse-ssh-key-fingerprints)). Ключ в секрете — приватный PEM без парольной фразы (`BEGIN … PRIVATE KEY`). Публичная половина добавляется в профиль GitVerse: Настройки → Ключи SSH/GPG.

Если заданы и токен, и ключ, по умолчанию используется HTTPS. Переменная `GITVERSE_MIRROR_AUTH=ssh` переключает на ключ. Если выбранный секрет пуст, а второй задан, скрипт берёт второй и пишет об этом. Если пусты оба, job успешна и ничего не отправляет: в логе и в summary есть предупреждение `Mirror skipped`.

Workflow срабатывает только в `DirektorBani/DataSafeS3` на `https://github.com`. Форк и случайный запуск того же файла на раннере GitVerse push не делают.

## CI на GitVerse

Файл [`.gitverse/workflows/ci.yaml`](../../.gitverse/workflows/ci.yaml). Каталог workflow — `.gitverse/workflows/`; синтаксис совместим с GitHub Actions ([быстрый старт](https://gitverse.ru/docs/cicd/quick-start), [синтаксис](https://gitverse.ru/docs/cicd/docs/workflow/)). Раннер также умеет читать `.github/workflows/`, поэтому в настройках репозитория нужен приоритет каталога `.gitverse/workflows/`. Точные подписи пункта меню документация не приводит: Настройки → Репозиторий → тумблер CI/CD → «приоритет для пути конфигурационных файлов» → Обновить ([вкладка репозитория](https://gitverse.ru/docs/collaborative/repositories/guides/repo-settings/repository-subtab-uc)).

Запуски смотреть на вкладке CI/CD репозитория. У публичных запусков адрес вида `https://gitverse.ru/DirektorBani/DataSafeS3/cicd/<номер>` (так устроены страницы запусков на GitVerse).

Триггеры `push` и `pull_request`. Раннер `ubuntu-latest`. `actions/checkout@v4` есть в быстром старте. `actions/setup-go@v5` уже выполняется на облачных раннерах других репозиториев; страница действия — <https://gitverse.ru/actions/setup-go>. `actions/setup-node` опубликован на <https://gitverse.ru/actions/setup-node>. Публичный успешный лог с меткой `@v4` не найден; версия та же, что в GitHub CI, а [FAQ](https://gitverse.ru/docs/cicd/faq/) говорит, что большинство действий Marketplace подходят.

| Job | Что делает |
| --- | --- |
| `go` | `gofmt -l`, `go vet ./...`, `go build ./...`, `go test ./...`, `govulncheck ./...`. Go `1.26.10`. |
| `web` | `npm ci` и `npm run build` в `web/console`, Node 24. |
| `repo-checks` | версии релиза (`check-release-pins.sh`), пути compose-оверлеев, ссылки на образы, проверка release workflow. |
| `single-node` | `python3 scripts/single-node-gate.py self-test`. Это проверки порогов и разбора вывода, без Docker. |

`go test ./...` не получает `TEST_POSTGRES_DSN`, `KAFKA_INTEGRATION`, `VAULT_ADDR`, `MINIO_ENDPOINT`. Тесты, которым нужна Postgres, пропускаются сами. Сервисных контейнеров в справочнике синтаксиса нет, а у облачного раннера нет `docker.sock` ([облачные раннеры](https://gitverse.ru/docs/cicd/docs/runners/cloud-hosted)), поэтому Postgres в этом workflow не поднимается.

Лимит одной job: [страница лимитов CI/CD](https://gitverse.ru/docs/cicd/docs/limits) называет 30 минут, [настройки раннеров](https://gitverse.ru/docs/repositories/repo-settings/runners-subtab-uc/) — 15 минут. В workflow стоит `timeout-minutes: 20` у `go` и `web` и `10` у `single-node`. Сборочное время: 1000 минут в месяц на публичный репозиторий и 500 на приватный (у верифицированной организации 2000 и 1000). Первый push всех веток и тегов может запустить CI на каждую обновлённую ссылку.

Секреты и переменные на стороне GitVerse этому CI не нужны. Имена секретов GitVerse не могут начинаться с `GITVERSE_` ([секреты](https://gitverse.ru/docs/cicd/docs/secrets-and-variables)).

### Что не выполняется в облачном CI GitVerse

Тяжёлые job из `.github/workflows/ci.yml` (необязательный набор на стороне зеркала) на облачный раннер не переносятся: нет демона Docker. Образы релиза при этом собираются без Docker ([build.md](build.md)). `install-ps1-syntax` требует `windows-latest`, такого раннера в документации облака нет.

| Job в `ci.yml` | Почему не в облачном CI GitVerse |
| --- | --- |
| `docker-build` | нет `docker.sock` |
| `single-node-gate` | Docker, образы, compose; на GitVerse остаётся только `self-test` |
| `e2e-smoke` | compose и Playwright |
| `kafka-integration`, `vault-integration`, `minio-migrate` | брокер, Vault и MinIO в Docker |
| `list-buckets-clients`, `upgrade-16`, `ha-promote-drill`, `cluster-kms-gate` | Docker и долгий стенд |
| `s3-compat` | набор клиентов и длинный прогон |
| `helm-smoke` | kind и кластер |
| `linux-packages` | QEMU и контейнеры дистрибутивов |
| `install-ps1-syntax` | раннер Windows |
| `event-observability` | в обязательный набор облака не входит |

## Секреты и переменные

Значения сюда не записываются. Секрет релиза создаётся на GitVerse: Настройки репозитория → CI/CD → Секреты. Остальные строки таблицы относятся к прежнему пути через GitHub (запасной).

| Имя | Где | Назначение |
| --- | --- | --- |
| `REGISTRY_TOKEN` | секрет GitVerse | Токен пользователя `DirektorBani` с записью в пакеты и в репозиторий (релизы). Нужен `.gitverse/workflows/release.yaml`. Имя не может начинаться с `GITVERSE_`. |
| `GITVERSE_TOKEN` | секрет GitHub (запасной путь) | Токен пользователя `DirektorBani`. Для зеркала нужен чекбокс «Репозитории». Для пакетов — запись (и удаление, если включена очистка). |
| `GITVERSE_SSH_KEY` | секрет GitHub | Приватный ключ без пароля. Нужен только при `GITVERSE_MIRROR_AUTH=ssh` или если токена нет. |
| `GITVERSE_MIRROR_AUTH` | переменная GitHub | `https` (по умолчанию) или `ssh`. |
| `GITVERSE_MIRROR_FORCE` | переменная GitHub | `true` включает форс-пуш на обычном push. Пусто или `false` — только fast-forward. |
| `GITVERSE_ARCHITECTURES`, `GITVERSE_DELETE_OLD`, `GITVERSE_KEEP_VERSIONS`, `GITVERSE_SIZE_LIMIT_MB`, `GITVERSE_DEB_DISTRIBUTION`, `GITVERSE_DEB_COMPONENT`, `GITVERSE_RPM_GROUP` | переменные GitHub | Только публикация пакетов, см. ниже. |

Для `ci.yaml` секреты не нужны. Для `release.yaml` нужен секрет `REGISTRY_TOKEN`. CI/CD должен быть включён, приоритет пути — `.gitverse/workflows/`.

### Что сделать владельцу

1. Убедиться, что `https://gitverse.ru/DirektorBani/DataSafeS3` — обычный репозиторий, а не импортированное зеркало. Иначе CI/CD недоступен, а push зеркала может быть запрещён.
2. Настройки → Репозиторий: включить CI/CD, выбрать приоритет `.gitverse/workflows/` (чтобы раннер не выполнял `.github/workflows/` с Docker), нажать Обновить.
3. Для git push выпустить токен с чекбоксом «Репозитории». Если текущий `GITVERSE_TOKEN` уже пишет пакеты и UI не даёт добавить этот чекбокс в тот же токен, оставить его для пакетов и настроить SSH: публичный ключ в GitVerse, приватный в секрет `GITVERSE_SSH_KEY`, переменная `GITVERSE_MIRROR_AUTH=ssh`.
4. Форс-пуш не включать, пока push не остановится на non-fast-forward. Тогда либо Actions → Mirror GitVerse → Run workflow → `force`, либо переменная `GITVERSE_MIRROR_FORCE=true`.
5. Создать секрет `REGISTRY_TOKEN` (см. выше). Без него `release.yaml` остановится на входе в реестр.
6. После первого успешного прогона открыть вкладку CI/CD на GitVerse и проверить job `go`, `web`, `repo-checks`, `single-node`, а для тега — `checks`, `images`, `packages`, `sbom`.

## Установка из реестров

Артефакты релиза DataSafeS3 публикуются в реестры пакетов [GitVerse](https://gitverse.ru/DirektorBani/DataSafeS3) (владелец `DirektorBani`). Так дистрибутив доступен с ресурса на территории РФ. Сборка и публикация идут на GitVerse: [build.md](build.md).

Тег образа совпадает с git-тегом (`v1.7.1`) и дополнительно ставится `latest`. Имена тегов в реестре GitVerse регистронезависимые; ниже они записаны строчными буквами.

## Docker

```bash
docker pull gitverse.ru/direktorbani/datasafe-storage-server:v1.7.1
docker pull gitverse.ru/direktorbani/datasafe-console:v1.7.1
```

Тег `latest` указывает на тот же релиз:

```bash
docker pull gitverse.ru/direktorbani/datasafe-storage-server:latest
docker pull gitverse.ru/direktorbani/datasafe-console:latest
```

Образ сервера собран для `linux/amd64` и `linux/arm64`. Образ консоли содержит статические файлы в `/srv/console`; чарт Kubernetes копирует их init-контейнером.

Compose из этого репозитория:

```bash
DATASAFE_SERVER_IMAGE=gitverse.ru/direktorbani/datasafe-storage-server:v1.7.1 \
DATASAFE_CONSOLE_IMAGE=gitverse.ru/direktorbani/datasafe-console:v1.7.1 \
docker compose up -d
```

Дальше — [первый запуск](../getting-started/ru/first-run.md).

Документация реестра: <https://gitverse.ru/docs/artifactory/packages-uc/docker-packages-uc>.

## Пакеты DEB

Пакет не привязан к кодовому имени Debian или Ubuntu, поэтому в реестре он лежит в дистрибутиве `stable` и компоненте `main`. Архитектуры: `amd64` и `arm64`.

```bash
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://gitverse.ru/api/packages/DirektorBani/debian/repository.key \
  -o /etc/apt/keyrings/gitverse-DirektorBani.asc
echo "deb [signed-by=/etc/apt/keyrings/gitverse-DirektorBani.asc] https://gitverse.ru/api/packages/DirektorBani/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/gitverse-DirektorBani.list
sudo apt update
sudo apt install datasafes3
```

Конкретная версия:

```bash
sudo apt install datasafes3=1.7.1-1
```

Документация: <https://gitverse.ru/docs/artifactory/packages-uc/deb-packages-uc>.

## Пакеты RPM

Группа репозитория — `stable`. Архитектуры: `x86_64` и `aarch64`.

```bash
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo
sudo dnf install datasafes3 --nogpgcheck
```

Конкретная версия:

```bash
sudo dnf install datasafes3-1.7.1-1.x86_64 --nogpgcheck
```

Пакет не подписан GPG. Документация GitVerse для этого случая указывает `--nogpgcheck`. Если реестр закрыт, в URL подставляют логин и токен:

```bash
sudo dnf config-manager --add-repo \
  https://<username>:<token>@gitverse.ru/api/packages/DirektorBani/rpm/stable.repo
```

Документация: <https://gitverse.ru/docs/artifactory/packages-uc/rpm-packages-uc/>.

## Helm

```bash
helm repo add datasafes3 https://gitverse.ru/api/packages/DirektorBani/helm
helm repo update
helm install ds datasafes3/datasafes3 --version 1.7.1 \
  --set storageServer.image.repository=gitverse.ru/direktorbani/datasafe-storage-server \
  --set console.image.repository=gitverse.ru/direktorbani/datasafe-console \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Пустой `storageServer.image.tag` в чарте означает `v` плюс `appVersion`, для этого релиза — `v1.7.1`. В чарте из дерева репозитория по умолчанию образы GitVerse.

Если реестр запрашивает авторизацию:

```bash
helm repo add datasafes3 \
  --username DirektorBani --password '<token>' \
  https://gitverse.ru/api/packages/DirektorBani/helm
```

Документация: <https://gitverse.ru/docs/artifactory/packages-uc/helm-packages-uc/>.

## После установки пакета

Сервис при установке не запускается: в `/etc/datasafes3/datasafes3.env` стоят заглушки секретов. Дальше как в разделе [пакетов Linux](../linux-packages.md#по-русски):

```bash
sudoedit /etc/datasafes3/datasafes3.env
sudo systemctl enable --now datasafes3
curl -sS http://127.0.0.1:9000/healthz
```

## Что публикует workflow

Основной путь — [`.gitverse/workflows/release.yaml`](../../.gitverse/workflows/release.yaml): образы (`linux/amd64`, `linux/arm64`), `.deb`, `.rpm`, чарт, страница релиза с `SHA256SUMS.txt`, дайджестами образов и SBOM. Подробности — [build.md](build.md). Ниже описан запасной путь через GitHub.

Ранее `.github/workflows/publish-gitverse.yml` (удалён; теперь всё делает `.gitverse/workflows/release.yaml`) копировал мультиархитектурный манифест (`linux/amd64` и `linux/arm64`) и загружает `.deb`, `.rpm` и чарт `datasafes3`. Суммарный лимит всех пакетов пользователя на GitVerse — 500 Мбайт. Перед загрузкой workflow считает размер. Если полный набор не помещается, ручной запуск с `architectures=amd64` (или переменная репозитория `GITVERSE_ARCHITECTURES=amd64`) публикует только amd64.

Очистка старых версий выключена по умолчанию. Её включает `delete_old` у `workflow_dispatch` или переменная `GITVERSE_DELETE_OLD=true`. Остаются две последние версии (`keep` / `GITVERSE_KEEP_VERSIONS`); тег `latest` не удаляется.

Ветки и теги исходного кода этот workflow не копирует. Их отправляет `mirror-gitverse`, раздел «Зеркало исходного кода» выше.
