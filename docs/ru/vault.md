# HashiCorp Vault для локальной разработки

English: [deploy/vault/README.md](../../deploy/vault/README.md). Руководство эксплуатации: [подстановка секретов](../operations-guide/ru/secrets-vault.md) и [Vault Transit](../operations-guide/ru/kms-vault.md).

Необязательный стек: Vault Agent рисует `STORAGE_*` в `/rendered/datasafe.env`, и контейнер читает файл при старте. Отдельного SDK Vault у DataSafeS3 нет.

Пока адрес не задан, процесс Vault не открывает. Это не часть установки одного узла.

## Что нужно

- Docker Compose v2
- `.env` с `DATASAFE_DATA_ROOT` (на Windows в примере локальной разработки — `D:/datasafe-data`)
- Каталоги `storage` и `postgres` под корнем данных, если включён профиль `postgres`

## Windows

```powershell
Copy-Item deploy/compose/env/.env.vault.example .env -ErrorAction SilentlyContinue
.\deploy\vault\local\setup-vault-dev.ps1
docker compose -p datasafe `
  -f docker-compose.yml `
  -f docker-compose.local-data.yml `
  -f deploy/compose/docker-compose.vault.yml `
  --profile vault up -d
```

## Linux и macOS

```bash
chmod +x deploy/vault/local/setup-vault-dev.sh deploy/vault/init-kv.sh
./deploy/vault/local/setup-vault-dev.sh
docker compose -p datasafe \
  -f docker-compose.yml \
  -f deploy/compose/docker-compose.vault.yml \
  --profile vault up -d
```

Профиль `vault` нужно указать явно. Обычный `docker compose up` этот стек не поднимает.

Строгие секреты, близкие к production, добавляют оверлей `deploy/compose/docker-compose.vault-product.yml` к тем же файлам.

Дымовая проверка не занимает порты основного стека: проект Compose `datasafe-vault`, хранилище **9001**, PostgreSQL **5434**.

```powershell
$env:DATASAFE_DATA_ROOT = 'D:/datasafe-data'
pwsh -File scripts/vault/smoke-vault-integration.ps1
```

```bash
DATASAFE_DATA_ROOT=./.datasafe-data scripts/vault/smoke-vault-integration.sh
```

Проверяются здоровье Vault, пути KV, `/healthz`, вход администратора с подставленным паролем и `security-status` с пустым `weak_secrets`.

В режиме разработки Vault слушает `http://localhost:8200`, токен dev-сервера — `root`. В бой этот режим не берут. Для боя — отдельный Vault, Kubernetes auth или AppRole и пример Agent Injector в `deploy/helm/datasafe/examples/values-vault-agent.yaml`.

| Служба | Профиль | Роль |
|--------|---------|------|
| `vault` | `vault` | Vault `server -dev` |
| `vault-init` | `vault` | Один запуск `init-kv.sh` в `secret/datasafe/*` |
| `vault-agent` | `vault` | Рисует `/rendered/datasafe.env` |
| `storage-server` | обычный | `entrypoint-with-vault-env.sh`, когда оверлей подключён |

Шифрование объектов через Transit — другая настройка (`STORAGE_KMS_PROVIDER=vault-transit`). Она описана в руководстве KMS и тоже молчит, пока адрес пуст.
