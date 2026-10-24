# Пакеты Linux

English: [linux-packages.md](../linux-packages.md).

Пакеты `datasafes3` для `linux/amd64` и `linux/arm64` опубликованы в реестрах пакетов [GitVerse](https://gitverse.ru/DirektorBani/DataSafeS3/packages) (владелец `DirektorBani`). Репозитории публичные, для установки вход не нужен.

| Формат | Репозиторий | Куда |
|--------|-------------|------|
| DEB (`amd64`, `arm64`) | `https://gitverse.ru/api/packages/DirektorBani/debian`, дистрибутив `stable`, компонент `main` | Debian, Ubuntu |
| RPM (`x86_64`, `aarch64`) | `https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo` | Fedora, RHEL, Rocky, Alma |

Репозиторий apt подписан GPG-ключом (`https://gitverse.ru/api/packages/DirektorBani/debian/repository.key`), `apt` проверяет подпись. Сами RPM-пакеты GPG не подписаны, поэтому `dnf` нужен `--nogpgcheck` (так документация GitVerse советует поступать с неподписанными пакетами); метаданные репозитория подписаны.

На странице релиза GitVerse для каждого тега лежит `SHA256SUMS.txt` для четырёх пакетов. Скачайте файлы пакетов и сверьте суммы:

```bash
sha256sum -c SHA256SUMS.txt
```

Подпись cosign для пакетов не публикуется; проверка — это GPG-подпись apt, контрольные суммы выше и TLS до `gitverse.ru`. См. [политику безопасности](security-policy.md).

## Что ставит пакет

- `/usr/bin/datasafe-storage-server` и `/usr/bin/storage-cli`
- `/usr/lib/datasafe/datasafe-kafka-bridge` — стартует только при заданном `STORAGE_KAFKA_BROKERS`
- системные пользователь и группа `datasafes3`, оболочка nologin
- `/etc/datasafes3/datasafes3.env`, режим `0640`, root:`datasafes3`. Это конфигурационный файл: обновление пакета сохраняет правки
- `/var/lib/datasafes3`, режим `0750`, владелец `datasafes3`. Здесь Bolt и файлы объектов
- unit systemd `datasafes3.service`

Unit при установке не включается и процесс не стартует. У Fedora так принято для необязательной службы. Debian обычно запускает демон сам. Этот пакет на обоих семействах остаётся остановленным: в файле окружения исторические секреты, и `STORAGE_STRICT_SECRETS=true` завершает процесс, пока их не заменят.

## Установка

Debian или Ubuntu:

```bash
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://gitverse.ru/api/packages/DirektorBani/debian/repository.key \
  -o /etc/apt/keyrings/gitverse-DirektorBani.asc
echo "deb [signed-by=/etc/apt/keyrings/gitverse-DirektorBani.asc] https://gitverse.ru/api/packages/DirektorBani/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/gitverse-DirektorBani.list
sudo apt update
sudo apt install datasafes3
```

Fedora, RHEL, Rocky или Alma:

```bash
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo
sudo dnf install datasafes3 --nogpgcheck
```

Конкретная версия: `sudo apt install datasafes3=1.7.1-1` или `sudo dnf install datasafes3-1.7.1-1`. Архитектура подбирается сама.

Подробности про реестры GitVerse: [gitverse-mirror.md](gitverse-mirror.md).

## Включение

Замените `STORAGE_ADMIN_PASSWORD`, `STORAGE_SECRET_KEY` и `STORAGE_JWT_SECRET`. Задайте `STORAGE_METRICS_TOKEN`, если до порта 9000 может достучаться не только localhost.

```bash
sudoedit /etc/datasafes3/datasafes3.env
sudo systemctl enable --now datasafes3
sudo systemctl status datasafes3
curl -sS http://127.0.0.1:9000/healthz
```

`storage-cli` ходит в этот API и своей службы не имеет. `DATASAFE_ENDPOINT` по умолчанию `http://127.0.0.1:9000`.

Журнал:

```bash
journalctl -u datasafes3 -f
```

Веб-консоль пакет отдельно не публикует. Swagger процесса: `http://127.0.0.1:9000/api/v1/docs`. Консоль на порту 8080 дают Compose и чарт.

## Удаление

```bash
sudo apt remove datasafes3
# или
sudo dnf remove datasafes3
```

`/var/lib/datasafes3` остаётся, включая `metadata.db` и файлы объектов. `apt purge datasafes3` забирает файл окружения, когда вы просите удалить конфигурацию пакета, и всё равно оставляет каталог данных. Удаляйте `/var/lib/datasafes3` сами, только если бакеты больше не нужны.

## Обновление

Выполните `sudo apt update && sudo apt install --only-upgrade datasafes3` (или `sudo dnf upgrade datasafes3 --nogpgcheck`). Файл окружения сохраняется. Если служба уже работала, пакет её перезапускает. Если была остановлена — остаётся остановленной.
