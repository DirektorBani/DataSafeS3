# Политика безопасности

**Продукт:** DataSafeS3  
Английский текст с теми же правилами раскрытия: [SECURITY.md](../../SECURITY.md).

Интерфейс консоли доступен на русском и английском. Эта страница не заменяет [порядок поддержки](support.md): сюда относятся только уязвимости.

---

## 1. Какие версии сопровождаются

| Версия | Сопровождение |
|--------|----------------|
| Последний тег релиза | да |
| Ветка `main` | по возможности |

Номер в дереве на момент документа: **1.7.0**. Следующие исправления цепочки сборки до выхода тега записаны в разделе `Unreleased` журнала изменений.

---

## 2. Как сообщить об уязвимости

Публичный issue для уязвимости не открывайте (ни на GitVerse, ни на зеркале GitHub).

1. Напишите на **trachyk.i@gmail.com** (или, на зеркале GitHub, создайте отчёт в [GitHub Security Advisories](https://github.com/DirektorBani/DataSafeS3/security/advisories/new)). В письме:
   - компонент и версия;
   - шаги, как повторить;
   - последствия, если они уже ясны.
2. Подтверждение, что сообщение получено, — в течение **3 рабочих дней**. Срок исправления по подтверждённой проблеме называют в течение **14 дней**.
3. Публичные детали предпочтительно не выходят **90 дней**, если исправление не готово раньше.

Адрес общих вопросов сопровождения в эту политику намеренно не включён, чтобы отчёт об уязвимости не смешивался с обычной перепиской. Он указан в [support.md](support.md).

---

## 3. Как проверить, что вы ставите выпущенный образ

Образы релиза опубликованы на GitVerse (`gitverse.ru/direktorbani/datasafe-storage-server` и `gitverse.ru/direktorbani/datasafe-console`, реестр публичный). Их собирает и подписывает GitHub Actions. Подписи [cosign](https://docs.sigstore.dev/) (keyless, OIDC) и SBOM привязаны к копии в GHCR, а манифесты платформ (`linux/amd64`, `linux/arm64`) на GitVerse имеют те же дайджесты, что в GHCR. Перед выкладкой проверьте копию в GHCR:

```bash
export COSIGN_EXPERIMENTAL=1
TAG=v1.7.0

cosign verify "ghcr.io/direktorbani/datasafe-storage-server:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com

cosign verify "ghcr.io/direktorbani/datasafe-console:${TAG}" \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com
```

Чтобы сравнить дайджесты, выполните `crane manifest <образ>:${TAG}` для имён GitVerse и GHCR и убедитесь, что записи `linux/amd64` и `linux/arm64` совпадают.

SBOM (`sbom-storage-server.cdx.json`, `sbom-console.cdx.json`) лежит на странице релиза на зеркале GitHub.

Пакеты `.deb` и `.rpm` в релизе на зеркале GitHub подписаны как blob (те же пакеты ставятся из репозиториев apt и dnf на GitVerse, см. [пакеты Linux](linux-packages.md)). `SHA256SUMS` покрывает четыре пакета и имеет свой bundle.

```bash
cosign verify-blob --bundle datasafes3_1.7.0_amd64.deb.bundle \
  --certificate-identity-regexp='https://github.com/DirektorBani/DataSafeS3/.+' \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  datasafes3_1.7.0_amd64.deb

sha256sum -c SHA256SUMS
```

Для файла, который ставите, берите bundle с тем же именем. Шаги установки: [linux-packages.md](linux-packages.md).

Репозитории GitVerse отдают те же пакеты. Проверка cosign относится к артефакту, а не к имени реестра: сверяйте подпись до установки. Репозиторий apt подписан GPG-ключом (`https://gitverse.ru/api/packages/DirektorBani/debian/repository.key`), `apt` проверяет подпись сам. RPM-пакеты GPG не подписаны, поэтому для `dnf` указывают `--nogpgcheck` (метаданные репозитория подписаны); контрольную сумму и bundle из релиза на зеркале GitHub при необходимости проверяют отдельно.

---

## 4. Как устроена разработка

- К образцам релиза прилагаются SBOM и подпись cosign.
- CI гоняет тесты, `govulncheck` и регрессионные гейты.
- Зависимости зафиксированы в `go.mod` и `package-lock.json`.
- Чеклист внешней оценки: [подготовка к испытаниям](../operations-guide/ru/pen-test-preparation.md).

### Go 1.26.10

`govulncheck` на Go 1.26.9 сообщает [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809): недоверие скомпрометированным промежуточным сертификатам в `crypto/x509`, исправлено в Go 1.26.10. Вызов прослеживается как `webauthn.FinishLogin` → `x509.Certificate.Verify`. Пины текущей сборки: `go.mod` `go 1.26.10`, образ `golang:1.26.10-alpine`, `setup-go` в CI, релизе и аудите. Запись лежит в `Unreleased`, пока не вышел следующий тег.

### Что уже закрывали предыдущие теги

- **1.4.1** (2026-10-11) перевёл цепочку на Go 1.26.9 и `golang.org/x/text` v0.41.0 и закрыл набор GO-2026-66xx, который `govulncheck` показывал на Go 1.26.8.
- **1.1.0** убрал `STORAGE_OUTBOUND_HTTP_ALLOW`, закрыл `/metrics` токеном `STORAGE_METRICS_TOKEN`, хранит токены ссылок только в виде хэша.
- **1.0.2** закрыл замечания по SSRF, токену OIDC в URL и секретам по умолчанию.

Подробности и шаги перехода: [руководство по обновлению](../operations-guide/ru/upgrade.md) и [CHANGELOG.md](../../CHANGELOG.md).

---

## 5. Что входит в Community

Функции безопасности этой линии (лаборатория HA, Object Lock, журнал, задел STS) поставляются по Apache-2.0 и не закрываются лицензионным ключом.

Минимальная установка не открывает Kafka и Vault. Пустые переменные не создают клиент. Секрет, который процесс не должен показывать (идентификатор секрета Vault, токен, секрет ключа доступа), не возвращается настройками, журналом и `/healthz`.

---

## 6. Что сделать оператору после обновления безопасности

1. Обновить сервер и консоль одним тегом.
2. Открыть `GET /api/v1/settings/security-status` и сменить отмеченные секреты.
3. Проверить исходящие URL вебхуков и журналов.
4. Не публиковать `/metrics` на внешнем прокси. В production Compose Caddy этот путь не отдаёт.
