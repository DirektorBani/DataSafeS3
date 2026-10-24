# Сборка и публикация релиза

Релиз собирается и публикуется на GitVerse. Для выпуска не нужны GitHub, GHCR и Docker Hub. Прежний путь через GitHub (`.github/workflows/publish-gitverse.yml`) остаётся запасным, пока новый путь не пройдёт полный прогон на GitVerse.

## Что получается на выходе

| Артефакт | Где лежит |
| --- | --- |
| Образ сервера хранения | `gitverse.ru/direktorbani/datasafe-storage-server:vX.Y.Z` и `latest`, `linux/amd64` + `linux/arm64` |
| Образ консоли | `gitverse.ru/direktorbani/datasafe-console:vX.Y.Z` и `latest`, `linux/amd64` + `linux/arm64` |
| Пакеты `.deb` / `.rpm` (amd64, arm64) | реестры apt и dnf GitVerse (`https://gitverse.ru/api/packages/DirektorBani/...`), см. [gitverse-mirror.md](gitverse-mirror.md) |
| Чарт Helm `datasafes3` | реестр Helm GitVerse |
| Страница релиза | `SHA256SUMS.txt` пакетов, `image-digests.txt`, `sbom-X.Y.Z.tgz` (CycloneDX), `THIRD_PARTY_LICENSES.md` |

Расширения `.deb` и `.rpm` страница релиза GitVerse не принимает, поэтому сами пакеты лежат в реестрах пакетов, а на странице релиза их контрольные суммы.

## Как запускается

Workflow [`.gitverse/workflows/release.yaml`](../../.gitverse/workflows/release.yaml) срабатывает на тег `vX.Y.Z` (и вручную, с указанием тега). Перед тегом `VERSION` и все закреплённые версии должны совпадать: `scripts/bump-release-version.sh X.Y.Z`, затем `scripts/check-release-pins.sh`.

Облачная job на GitVerse ограничена по времени (15–30 минут в зависимости от страницы документации), общего хранилища между job нет, поэтому работа разбита:

| Job | Что делает |
| --- | --- |
| `checks` | тег совпадает с `VERSION`, закреплённые версии, пути compose-оверлеев, ссылки на образы, проверка самого workflow, `go vet`, `go build` |
| `images` | `scripts/build-images.sh`: собирает Go-бинарники для обеих архитектур, консоль (`npm`), собирает образы и отправляет в реестр |
| `packages` | `scripts/package-linux.sh` (`.deb`, `.rpm`), `helm lint`, `scripts/gitverse-publish-packages.sh` (apt, dnf, Helm), контрольные суммы на страницу релиза |
| `sbom` | `scripts/sbom.sh`: SBOM модулей Go, npm-зависимостей консоли и обоих образов; дайджесты образов; всё это на страницу релиза |

Секрет `REGISTRY_TOKEN` (Настройки репозитория → CI/CD → Секреты): токен пользователя `DirektorBani` с записью в пакеты и в репозиторий. Имена секретов GitVerse не могут начинаться с `GITVERSE_`. Значение в логи не попадает: вход в реестр пишет файл с правами `0600`, остальные скрипты передают токен через файл заголовков.

## Как собираются образы без Docker

На облачном раннере GitVerse нет `docker.sock`, а эмуляция другой архитектуры недоступна. Поэтому:

1. Go кросс-компилирует бинарники (`CGO_ENABLED=0`, `GOARCH` из `TARGETARCH`): `storage-server`, `datasafe-kafka-bridge`, `su-exec` (собственный статический аналог, [cmd/su-exec](../../cmd/su-exec)).
2. Консоль собирается `npm ci && npm run build`.
3. [tools/imgbuild](../../tools/imgbuild) (go-containerregistry) берёт базовый образ Alpine нужной архитектуры, добавляет слои с бинарниками и скриптами и отправляет индекс образов в реестр.

В финальных стадиях `deploy/docker/Dockerfile` и `Dockerfile.console` нет `RUN`, только `COPY`, поэтому образ из `docker build` и образ из `imgbuild` одинаковы по содержимому. Пакеты в образ не устанавливаются: `ca-certificates-bundle` и busybox `wget` для healthcheck уже есть в Alpine.

Локально то же самое: `scripts/build-images.sh --registry gitverse.ru/direktorbani --tags v1.7.1,latest` (вход в реестр через `DOCKER_CONFIG`). Для проверки без публикации можно поднять локальный реестр (`crane registry serve`) и указать его в `--registry`.

## От чего зависит сборка

Всё берётся с GitVerse:

| Что | Откуда |
| --- | --- |
| Исходный код | `gitverse.ru/DirektorBani/DataSafeS3` |
| Модули Go | `GOPROXY=https://go-mirror.gitverse.ru,direct` |
| npm-пакеты | `https://npm-mirror.gitverse.ru/` |
| Базовый образ Alpine, образ `golang` для `docker build` | `dh-mirror.gitverse.ru/library/` |
| nfpm, helm, syft, cyclonedx-gomod, crane | `go install` через go-mirror |
| Готовые образы и пакеты | реестры GitVerse |

Честные оговорки:

- go-mirror, npm-mirror и dh-mirror — кеширующие зеркала публичных источников. Содержимое пакетов остаётся тем, что опубликовали авторы; хэши фиксируют `go.sum` и `package-lock.json`.
- Действия `actions/checkout`, `actions/setup-go`, `actions/setup-node` берутся с самой платформы GitVerse ([gitverse.ru/actions](https://gitverse.ru/actions)).
- Если зеркала GitVerse недоступны, сборка остановится, а не уйдёт на зарубежный адрес: переменные `GOPROXY` и `NPM_REGISTRY` можно переопределить на собственный прокси.
- Образы не подписываются (cosign не используется). Подлинность проверяется по дайджесту из `image-digests.txt` на странице релиза и по `SHA256SUMS.txt` пакетов.

## Чего workflow не умеет и что решает владелец

- Облачные раннеры GitVerse не запускают контейнеры. Интеграционные проверки со стендом (compose, Kafka, Vault, Playwright, установка пакетов в чистых ОС) в облаке не идут. Для них нужен свой раннер в РФ, решение о его установке остаётся за владельцем.
- Лимит сборочного времени облака — 1000 минут в месяц на публичный репозиторий.
- Суммарный лимит пакетов пользователя на GitVerse — около 500 Мбайт: `scripts/gitverse-publish.sh` (запасной путь) умеет считать размер и удалять старые версии; новый скрипт `scripts/gitverse-publish-packages.sh` повторный запуск переносит спокойно (409 считается успехом), но старые версии не удаляет.
- Первый прогон на GitVerse нужно проверить глазами: анонимный `crane manifest` обоих образов, метаданные apt/dnf/Helm, `helm template` с значениями по умолчанию.

## Ручная публикация (если облако недоступно)

```bash
export GOPROXY=https://go-mirror.gitverse.ru,direct
bash scripts/check-release-pins.sh
bash scripts/package-linux.sh
TAG=v1.7.1 GV_USER=DirektorBani GV_TOKEN=... PKG_DIR=dist/packages \
  CHART_SRC=deploy/helm/datasafes3 bash scripts/gitverse-publish-packages.sh
bash scripts/build-images.sh --registry gitverse.ru/direktorbani --tags v1.7.1,latest
TAG=v1.7.1 GV_TOKEN=... bash scripts/gitverse-release-assets.sh dist/packages/SHA256SUMS.txt
```

Токены в командную строку и в историю shell лучше не вставлять: используйте `read -s`.
