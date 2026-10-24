# Установка из зеркала GitVerse

Артефакты релиза DataSafeS3 публикуются в реестры пакетов [GitVerse](https://gitverse.ru/DirektorBani/DataSafeS3) (владелец `DirektorBani`). Так дистрибутив доступен с ресурса на территории РФ. Тот же релиз по-прежнему лежит на GitHub и в `ghcr.io`.

Тег образа совпадает с git-тегом (`v1.7.0`) и дополнительно ставится `latest`. Имена тегов в реестре GitVerse регистронезависимые; ниже они записаны строчными буквами.

## Docker

```bash
docker pull gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0
docker pull gitverse.ru/direktorbani/datasafe-console:v1.7.0
```

Тег `latest` указывает на тот же релиз:

```bash
docker pull gitverse.ru/direktorbani/datasafe-storage-server:latest
docker pull gitverse.ru/direktorbani/datasafe-console:latest
```

Образ сервера собран для `linux/amd64` и `linux/arm64`. Образ консоли содержит статические файлы в `/srv/console`; чарт Kubernetes копирует их init-контейнером.

Compose из этого репозитория:

```bash
DATASAFE_SERVER_IMAGE=gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0 \
DATASAFE_CONSOLE_IMAGE=gitverse.ru/direktorbani/datasafe-console:v1.7.0 \
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
sudo apt install datasafes3=1.7.0
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
sudo dnf install datasafes3-1.7.0-1.x86_64 --nogpgcheck
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
helm install ds datasafes3/datasafes3 --version 1.7.0 \
  --set storageServer.image.repository=gitverse.ru/direktorbani/datasafe-storage-server \
  --set console.image.repository=gitverse.ru/direktorbani/datasafe-console \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Пустой `storageServer.image.tag` в чарте означает `v` плюс `appVersion`, для этого релиза — `v1.7.0`. В чарте по умолчанию образы указывают на `ghcr.io`; параметры `--set` выше берут их из GitVerse.

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

[`.github/workflows/publish-gitverse.yml`](../../.github/workflows/publish-gitverse.yml) копирует мультиархитектурный манифест (`linux/amd64` и `linux/arm64`) и загружает `.deb`, `.rpm` и чарт `datasafes3`. Суммарный лимит всех пакетов пользователя на GitVerse — 500 Мбайт. Перед загрузкой workflow считает размер. Если полный набор не помещается, ручной запуск с `architectures=amd64` (или переменная репозитория `GITVERSE_ARCHITECTURES=amd64`) публикует только amd64.

Очистка старых версий выключена по умолчанию. Её включает `delete_old` у `workflow_dispatch` или переменная `GITVERSE_DELETE_OLD=true`. Остаются две последние версии (`keep` / `GITVERSE_KEEP_VERSIONS`); тег `latest` не удаляется.
