# Пакеты Linux

English: [linux-packages.md](../linux-packages.md).

Релиз GitHub `v1.7.0` содержит пакеты `datasafes3` для `linux/amd64` и `linux/arm64`:

| Файл | Куда |
|------|------|
| `datasafes3_1.7.0_amd64.deb` | Debian, Ubuntu (amd64) |
| `datasafes3_1.7.0_arm64.deb` | Debian, Ubuntu (arm64) |
| `datasafes3-1.7.0-1.x86_64.rpm` | Fedora, RHEL, Rocky, Alma (x86_64) |
| `datasafes3-1.7.0-1.aarch64.rpm` | Fedora, RHEL, Rocky, Alma (aarch64) |
| `SHA256SUMS` | Контрольные суммы этих четырёх файлов |

У каждого файла есть cosign-bundle с суффиксом `.bundle`. Сначала сумма, потом подпись:

```bash
sha256sum -c SHA256SUMS
cosign verify-blob --bundle datasafes3_1.7.0_amd64.deb.bundle \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  datasafes3_1.7.0_amd64.deb
```

Bundle должен соответствовать тому файлу, который ставите. Та же проверка личности сертификата описана в [политике безопасности](security-policy.md).

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
sudo apt install ./datasafes3_1.7.0_amd64.deb
```

Fedora, RHEL, Rocky или Alma:

```bash
sudo dnf install ./datasafes3-1.7.0-1.x86_64.rpm
```

На arm64 — deb `arm64` или rpm `aarch64`.

Те же пакеты в реестрах GitVerse: [gitverse-mirror.md](gitverse-mirror.md).

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

Новый пакет ставится той же командой. Файл окружения сохраняется. Если служба уже работала, пакет её перезапускает. Если была остановлена — остаётся остановленной.
