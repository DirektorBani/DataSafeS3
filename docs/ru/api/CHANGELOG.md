# Журнал интеграционного API

English: [api/CHANGELOG.md](../../api/CHANGELOG.md).

Номер в `info.version` файла OpenAPI. Это не номер продукта в `VERSION`. Текущая консоль говорит на русском и английском. Запись 1.1.0 ниже фиксирует, что тот релиз API добавил коды локали `de` и `fr`; языки интерфейса продукта сейчас — русский и английский.

## 1.1.0 — 2026-06-22

### Добавлено

- Конечные точки WebAuthn MFA (`/me/mfa/webauthn/*`, `/auth/mfa/webauthn/*`)
- События аудита ссылок (`share.created`, `share.downloaded` и другие)
- Проверка связи узла федерации (`POST /federation/clusters/{id}/test`)
- Поля `/healthz`: `read_only_mode`, `replication_lag_s`
- Коды локали `de` и `fr` на `PATCH /me/locale`

### Изменено

- Маршруты OpenAPI уровня A приведены к scope P0/P1

## 1.0.0

Первый Community Integration API (`docs/api/openapi.yaml`).
