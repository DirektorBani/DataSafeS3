# Linux packages

По-русски: [ru/linux-packages.md](ru/linux-packages.md).



The GitHub Release for `v1.7.0` includes `datasafes3` packages for `linux/amd64` and `linux/arm64`:

| File | Where |
|------|--------|
| `datasafes3_1.7.0_amd64.deb` | Debian, Ubuntu (amd64) |
| `datasafes3_1.7.0_arm64.deb` | Debian, Ubuntu (arm64) |
| `datasafes3-1.7.0-1.x86_64.rpm` | Fedora, RHEL, Rocky, Alma (x86_64) |
| `datasafes3-1.7.0-1.aarch64.rpm` | Fedora, RHEL, Rocky, Alma (aarch64) |
| `SHA256SUMS` | Checksums of those four files |

Each of those files also has a cosign bundle named with a `.bundle` suffix. Check the checksums, then the signature, before you install:

```bash
sha256sum -c SHA256SUMS
cosign verify-blob --bundle datasafes3_1.7.0_amd64.deb.bundle \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  datasafes3_1.7.0_amd64.deb
```

Use the bundle that matches the file you install. The same identity check is in [SECURITY.md](../SECURITY.md).

## What the package installs

- `/usr/bin/datasafe-storage-server` and `/usr/bin/storage-cli`
- `/usr/lib/datasafe/datasafe-kafka-bridge`, started only when `STORAGE_KAFKA_BROKERS` is set
- system user and group `datasafes3`, with a nologin shell
- `/etc/datasafes3/datasafes3.env`, mode `0640`, root:`datasafes3`. It is a config file, so a later package upgrade keeps your edits
- `/var/lib/datasafes3`, mode `0750`, owned by `datasafes3`. Bolt metadata and object files live here
- systemd unit `datasafes3.service`

The unit is not enabled and the process is not started during install. Fedora's packaging rule is to leave a non-essential service disabled. Debian would usually start a daemon. This package stays stopped on both, because the shipped env file still has the historical default secrets and `STORAGE_STRICT_SECRETS=true` exits until you replace them.

## Install

Debian or Ubuntu:

```bash
sudo apt install ./datasafes3_1.7.0_amd64.deb
```

Fedora, RHEL, Rocky, or Alma:

```bash
sudo dnf install ./datasafes3-1.7.0-1.x86_64.rpm
```

On arm64 use the `arm64` deb or the `aarch64` rpm.

## Enable

Edit the env file. Replace `STORAGE_ADMIN_PASSWORD`, `STORAGE_SECRET_KEY`, and `STORAGE_JWT_SECRET`. Set `STORAGE_METRICS_TOKEN` if anything besides localhost can reach port 9000.

```bash
sudoedit /etc/datasafes3/datasafes3.env
sudo systemctl enable --now datasafes3
sudo systemctl status datasafes3
curl -sS http://127.0.0.1:9000/healthz
```

`storage-cli` talks to that API. It has no service of its own. `DATASAFE_ENDPOINT` defaults to `http://127.0.0.1:9000`.

Logs:

```bash
journalctl -u datasafes3 -f
```

## Uninstall

```bash
sudo apt remove datasafes3
# or
sudo dnf remove datasafes3
```

`/var/lib/datasafes3` stays, including `metadata.db` and the object files. `apt purge datasafes3` removes the env file when you ask to drop package configuration, and still leaves the data directory. Delete `/var/lib/datasafes3` yourself only when you mean to drop the buckets.

## Upgrade

Install the newer package with the same command. Your env file is kept. If the service was already running, the package restarts it. If it was stopped, it stays stopped.

## По-русски

Пакеты `datasafes3` лежат на GitHub Release `v1.7.0`: `.deb` для Debian/Ubuntu и `.rpm` для Fedora, RHEL, Rocky и Alma, архитектуры amd64 и arm64. Рядом `SHA256SUMS` и cosign-bundle на каждый файл.

Пакет ставит бинарники `datasafe-storage-server` и `storage-cli`, мост Kafka, системного пользователя `datasafes3` без входа в систему, unit `datasafes3.service`, конфиг `/etc/datasafes3/datasafes3.env` и каталог данных `/var/lib/datasafes3`.

Сервис при установке не включается и не запускается. В файле окружения исторические секреты, и `STORAGE_STRICT_SECRETS=true` не даст процессу стартовать, пока вы их не замените. Debian обычно запускает демон сам, Fedora — нет. Этот пакет на обоих семействах остаётся остановленным: так не поднимается сервис с паролем по умолчанию.

```bash
sudo apt install ./datasafes3_1.7.0_amd64.deb
# или: sudo dnf install ./datasafes3-1.7.0-1.x86_64.rpm
sudoedit /etc/datasafes3/datasafes3.env
sudo systemctl enable --now datasafes3
```

Удаление пакета не удаляет `/var/lib/datasafes3`. Там остаются `metadata.db` и файлы объектов. `apt purge` забирает конфиг, каталог данных остаётся. Обновление пакета сохраняет ваши правки в env-файле.

Те же `.deb` и `.rpm` публикуются в реестры GitVerse: [установка из зеркала](ru/gitverse-mirror.md).
