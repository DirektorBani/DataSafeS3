# DataSafeS3 vs alternatives (RU)

See English version: [why-datasafes3-vs.md](../en/why-datasafes3-vs.md)

Сравнение Community Edition для сценариев **управляемого self-hosted S3**. Утверждения привязаны к регрессионным тестам и документации.

| Возможность | DataSafeS3 CE | MinIO CE | Nextcloud | SeaweedFS |
|-------------|-------------|----------|-----------|-----------|
| S3 API | Да | Да | Частично | Да |
| Object Lock XML | Да | Да | Нет | Ограничено |
| LDAP + OIDC | Да | Да | Сильно | Нет |
| WebAuthn MFA | Да | Ограничено | Через приложения | Нет |
| Share links + audit | Да | Нет | Сильно | Нет |

## Где мы не конкурируем

- Petabyte erasure / гипермасштаб
- Синхронизация файлов и коллаборация уровня Nextcloud

## Доказательства

- `scripts/feature-audit-test.ps1`
- [performance-benchmarks.md](../testing/performance-benchmarks.md)
