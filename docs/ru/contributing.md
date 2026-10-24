# Участие в разработке DataSafeS3

English: [CONTRIBUTING.md](../../CONTRIBUTING.md).

Спасибо, что улучшаете Community Edition. Интерфейс продукта доступен на русском и английском; пользовательскую документацию правят на обоих языках.

## Сборка и тесты

```powershell
go test ./...
cd web/console && npm ci && npm run build
```

### Локальный стек (Windows)

```powershell
docker compose -p datasafe --profile postgres `
  -f docker-compose.yml `
  -f docker-compose.local-data.yml `
  -f deploy/compose/docker-compose.local-binary.yml `
  up -d postgres storage-server caddy
scripts\feature-audit-test.ps1
```

`feature-audit-test.ps1` ждёт стек на порту 8080 и оверлей `deploy/compose/docker-compose.audit.yml`.

### Playwright

При поднятом Compose на `http://127.0.0.1:8080`:

```powershell
cd web/console
npx playwright install chromium
npx playwright test e2e/smoke.spec.ts e2e/buckets.spec.ts e2e/settings.spec.ts e2e/files.spec.ts e2e/share.spec.ts e2e/security-mfa.spec.ts e2e/teams.spec.ts
```

**OIDC Keycloak** не является гейтом PR. `E2E_OIDC_KEYCLOAK=1`, Keycloak (`scripts\start-keycloak-test.cmd`), затем `e2e/security-oidc-keycloak.spec.ts`. Политика флейков: [oidc-e2e.md](testing/oidc-e2e.md).

## Гейт перед тегом

1. Пересобрать бинарник для оверлея `local-binary`:  
   `$env:GOOS='linux'; $env:GOARCH='amd64'; go build -o deploy/docker/storage-server-linux ./cmd/storage-server`
2. Чистые тома: `docker compose ... down -v`, очистить каталоги данных storage и postgres.
3. Стек с `deploy/compose/docker-compose.audit.yml` (и Prometheus/Grafana, если нужны строки мониторинга).
4. `scripts\start-minio-test.cmd` (или автозапуск MinIO на порту 9100 из feature-audit).
5. Боковые контейнеры полного прогона: `start-ldap-test.cmd`, `start-keycloak-test.cmd`, `start-elasticsearch-test.cmd`, `start-loki-test.cmd`.
6. `scripts\feature-audit-test.ps1` — цель **0 FAIL**.
7. Семь spec Playwright на `http://127.0.0.1:8080`.
8. Дым Vault отдельно: `pwsh -File scripts/vault/smoke-vault-integration.ps1` — порты **9001** и **5434**, основной стек не занимает.

Шаблон отчёта QA лежит в `_local/qa/` и в git не коммитится.

Внутренние ТЗ — в `D:\datasafe_tz\specs\`, не в git. См. [specs/README.md](specs/README.md).

## Чеклист запроса на слияние

- [ ] `go test ./...` проходит
- [ ] Консоль собирается, если менялся интерфейс (`npm run build`)
- [ ] Регрессия Playwright, если менялись интерфейс или вход
- [ ] Feature-audit не сужен: новые проверки добавляют, старые не выкидывают молча
- [ ] Пользовательская документация обновлена на русском и английском, если изменилось поведение
- [ ] В коммите нет секретов и локальных черновиков дорожной карты
- [ ] Для крупной функции есть ревью владельца продукта

## С чего начать

В трекере ищите метку `good first issue`. Типичные темы: устойчивость `teams.spec.ts`, новая строка feature-audit, перевод примера из `docs/api-guide`.

Каталоги `docs/analysis/`, `_local/` и заполненные отчёты аудита в gitignore. Не коммитьте их.

## Community и Enterprise

- [Жизненный цикл Community и Enterprise](enterprise/community-enterprise-lifecycle.md)
- [Оценка запроса на функцию](enterprise/feature-request-evaluation.md)

## Стиль

Правка должна быть не шире задачи: те же имена, что вокруг, без новой абстракции «на будущее».

Дефект и идея, которые не являются уязвимостью, — через шаблон issue. Уязвимость — по [политике безопасности](security-policy.md), не через публичный issue.
