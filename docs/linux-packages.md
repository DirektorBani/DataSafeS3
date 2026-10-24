# Linux packages

По-русски: [ru/linux-packages.md](ru/linux-packages.md).



The `datasafes3` packages for `linux/amd64` and `linux/arm64` are published to the package repositories on [GitVerse](https://gitverse.ru/DirektorBani/DataSafeS3/packages) (owner `DirektorBani`). The repositories are public; no login is needed to install.

| Format | Repository | Where |
|--------|------------|-------|
| DEB (`amd64`, `arm64`) | `https://gitverse.ru/api/packages/DirektorBani/debian`, distribution `stable`, component `main` | Debian, Ubuntu |
| RPM (`x86_64`, `aarch64`) | `https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo` | Fedora, RHEL, Rocky, Alma |

The apt repository is signed with a GPG key (`https://gitverse.ru/api/packages/DirektorBani/debian/repository.key`) and `apt` verifies it. The RPM packages themselves are not GPG-signed, so `dnf` needs `--nogpgcheck` (as the GitVerse docs describe for unsigned packages); the repository metadata is signed.

The GitVerse release page for each tag carries `SHA256SUMS.txt` for the four packages. Download the package files (or read their checksums from the registry page) and compare:

```bash
sha256sum -c SHA256SUMS.txt
```

Packages are not signed with cosign; verification is the apt GPG signature, the checksums above and TLS to `gitverse.ru`. See [SECURITY.md](../SECURITY.md).

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
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://gitverse.ru/api/packages/DirektorBani/debian/repository.key \
  -o /etc/apt/keyrings/gitverse-DirektorBani.asc
echo "deb [signed-by=/etc/apt/keyrings/gitverse-DirektorBani.asc] https://gitverse.ru/api/packages/DirektorBani/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/gitverse-DirektorBani.list
sudo apt update
sudo apt install datasafes3
```

Fedora, RHEL, Rocky, or Alma:

```bash
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo
sudo dnf install datasafes3 --nogpgcheck
```

A specific version: `sudo apt install datasafes3=1.7.1-1` or `sudo dnf install datasafes3-1.7.1-1`. The architecture is picked automatically.

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

Run `sudo apt update && sudo apt install --only-upgrade datasafes3` (or `sudo dnf upgrade datasafes3 --nogpgcheck`). Your env file is kept. If the service was already running, the package restarts it. If it was stopped, it stays stopped.

## По-русски

Пакеты `datasafes3` лежат в реестрах пакетов [GitVerse](https://gitverse.ru/DirektorBani/DataSafeS3/packages): `.deb` для Debian/Ubuntu и `.rpm` для Fedora, RHEL, Rocky и Alma, архитектуры amd64 и arm64. Контрольные суммы пакетов лежат в `SHA256SUMS.txt` на странице релиза GitVerse.

Пакет ставит бинарники `datasafe-storage-server` и `storage-cli`, мост Kafka, системного пользователя `datasafes3` без входа в систему, unit `datasafes3.service`, конфиг `/etc/datasafes3/datasafes3.env` и каталог данных `/var/lib/datasafes3`.

Сервис при установке не включается и не запускается. В файле окружения исторические секреты, и `STORAGE_STRICT_SECRETS=true` не даст процессу стартовать, пока вы их не замените. Debian обычно запускает демон сам, Fedora — нет. Этот пакет на обоих семействах остаётся остановленным: так не поднимается сервис с паролем по умолчанию.

```bash
# Debian, Ubuntu
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://gitverse.ru/api/packages/DirektorBani/debian/repository.key \
  -o /etc/apt/keyrings/gitverse-DirektorBani.asc
echo "deb [signed-by=/etc/apt/keyrings/gitverse-DirektorBani.asc] https://gitverse.ru/api/packages/DirektorBani/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/gitverse-DirektorBani.list
sudo apt update && sudo apt install datasafes3

# Fedora, RHEL, Rocky, Alma (RPM не подписан GPG, поэтому --nogpgcheck)
sudo dnf install -y dnf-plugins-core
sudo dnf config-manager --add-repo https://gitverse.ru/api/packages/DirektorBani/rpm/stable.repo
sudo dnf install datasafes3 --nogpgcheck

sudoedit /etc/datasafes3/datasafes3.env
sudo systemctl enable --now datasafes3
```

Удаление пакета не удаляет `/var/lib/datasafes3`. Там остаются `metadata.db` и файлы объектов. `apt purge` забирает конфиг, каталог данных остаётся. Обновление пакета сохраняет ваши правки в env-файле.

Подробности про реестры GitVerse: [gitverse-mirror.md](ru/gitverse-mirror.md).
