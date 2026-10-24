# Что изменилось в 1.7.1

Полный журнал изменений на английском: [CHANGELOG.md](../../CHANGELOG.md). Ниже коротко, по-русски.

## 1.7.1 — 24.10.2026

Исправления по результатам аудита 1.7.0 и сборка релиза на GitVerse. Образы не подписываются (cosign не используется): подлинность проверяется по дайджесту, см. [security-policy.md](security-policy.md).

### Безопасность

- Go 1.26.10 (закрывает GO-2026-6809 в `crypto/x509`), `golang.org/x/crypto` v0.58.0, `go-ntlmssp` v0.1.1.
- Консоль: `zod`, `@hookform/resolvers`, `react-router-dom` обновлены; `npm audit --omit=dev` — 0 замечаний. Остаются замечания только к инструментам сборки (сервер разработки Vite/esbuild, Tailwind 3); в поставляемые файлы они не попадают, закрываются переходом на новые мажорные версии.
- `nats-server` остаётся в `go.mod` только для тестов и в бинарники релиза не входит.

### Исправлено

- **Образ arm64 в 1.7.0 содержал бинарник amd64** (в Dockerfile было жёстко `GOARCH=amd64`). Теперь каждая архитектура собирается со своим `GOARCH`.
- Консоль: переключатель языка только RU и EN (раньше показывал ещё DE и FR); форма входа больше не подставляет `admin` / `admin`; кнопка темы и профиль переведены; в мастере убран текст «external S3 S3».
- Мастер настройки: проверка S3 больше не падает с HeadBucket 404, пока нет корзины (например `dr-archive`).
- HA: резервный узел отвечает на запросы репликации объектов (`STORAGE_INTERNAL_ADDR` по умолчанию `:9002` при включённом HA); у резервных узлов есть токен Vault (оверлей `docker-compose.vault-ha.yml`).
- Compose: пути в `docker-compose.vault-kms.yml` больше не ведут в `../vault/...`.
- Grafana: на панели кластера нет красной строки `localhost:9000 down`, «Nodes configured» считает реальные узлы.
- Windows: `install.ps1` не падает с «property 'ldap' cannot be found»; `ensure-docker-pull-proxy.cmd` работает в русской Windows.
- Keycloak: `issuer` из discovery не сверяется с введённым адресом, подменяется только хост адресов; см. [integrations/ldap-keycloak-standalone.md](integrations/ldap-keycloak-standalone.md) (`KC_HOSTNAME`).

### Изменено

- Образы: Alpine 3.24, Node 24 для сборки консоли; в образ сервера не ставятся пакеты (healthcheck — busybox `wget`, `su-exec` — собственный статический бинарник). Образы по умолчанию и чарты Helm указывают на `gitverse.ru/direktorbani/...:v1.7.1`.
- Сборка и публикация релиза идут на GitVerse: [build.md](build.md).
- SBOM (CycloneDX) к каждому релизу, `THIRD_PARTY_LICENSES.md`, `Cargo.lock` настольного клиента.
- Поддержка: первый ответ на сообщение об уязвимости — 1 рабочий день (было 3); поддерживаются последний релиз и исправления безопасности для предыдущей минорной линии 12 месяцев; часы поддержки 9:00–21:00 МСК (UTC+3).

### Не проверено без работающего стенда

Сценарий PowerShell на Windows, `.cmd` в русской Windows, HA с Vault и отрисовка Grafana (проверены `docker compose config` и модульные тесты), поведение Keycloak с issuer, сквозные тесты Playwright.
