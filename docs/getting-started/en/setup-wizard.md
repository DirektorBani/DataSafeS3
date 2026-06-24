English | **[Русский](../ru/setup-wizard.md)**

# Initial setup wizard

On first admin login after a fresh install, DataSafeS3 redirects to `/setup` until initial configuration is complete.

## Flow

1. **Change password** — required before any other setup step
2. **Welcome** — overview of optional external S3 backup
3. **External S3** (optional) — connect external S3 for Gateway replication
4. **Finish** — skip S3 or save validated connection

## API status

```http
GET /api/v1/setup/status
```

Response flags: `needs_setup`, `needs_password_change`, `initial_setup_completed`.

## Skip external S3

After password change:

```http
POST /api/v1/setup/complete
Authorization: Bearer <token>
```

## Configure external S3

```http
POST /api/v1/setup/s3/save
```

Requires successful connection test. See [S3 configuration](s3-configuration.md).

## Specification

Full TZ: [../../en/specs/initial-setup-wizard-tz.md](../../en/specs/initial-setup-wizard-tz.md)
