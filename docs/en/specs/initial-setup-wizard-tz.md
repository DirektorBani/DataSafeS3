English | **[Русский](../../ru/specs/initial-setup-wizard-tz.md)**

# Spec: Initial Setup Wizard

**Version:** 1.0  
**Date:** 2026-06-20  
**Status:** Implemented  
**Related code:** `internal/api/setup_handlers.go`, `web/console/src/pages/setup.tsx`, `scripts/reset-fresh-install.ps1`

Full specification (Russian): **[initial-setup-wizard-tz.md](../../ru/specs/initial-setup-wizard-tz.md)**

---

## 1. Goal

On first install after a clean deployment, an administrator completes a short setup wizard (welcome + optional external S3). Until the wizard finishes, other console sections are blocked.

---

## 2. Reset to fresh install

Script `scripts/reset-fresh-install.ps1` (or `.cmd`):

- stops Docker Compose;
- deletes `metadata.db` and `objects/` under `STORAGE_DATA_DIR`;
- with `postgres` profile — `docker compose --profile postgres down -v`;
- restarts the stack.

---

## 3. Flow

| Step | Description |
|------|-------------|
| 1 | Clean DB: `initial_setup_completed=false`, `admin_first_login_completed=false`, `admin_password_changed=false` |
| 2 | Login page shows `admin / admin` hint while `admin_first_login_completed=false` |
| 3 | First successful admin login → `admin_first_login_completed=true`, redirect to `/setup` |
| 4 | **Mandatory password change** modal before wizard; `POST /me/password` → `admin_password_changed=true` |
| 5 | Welcome step — start S3 setup or **skip** (complete without S3) |
| 6 | S3 form: Endpoint, keys, bucket, region, SSL; Test, Save, or **Skip S3** |
| 7 | Finish (`POST /setup/s3/save` or `POST /setup/complete`) → `initial_setup_completed=true`, full access |
| 8 | Progress bar: Password → Welcome → S3 → Done |
| 9 | Wizard not shown on subsequent logins |

---

## 4. Data model (`SystemConfig`)

See JSON in [Russian spec](../../ru/specs/initial-setup-wizard-tz.md#4-модель-данных-systemconfig).

---

## 5. API

| Method | Path | Access | Description |
|--------|------|--------|-------------|
| GET | `/api/v1/setup/status` | public | Setup flags and `needs_setup` |
| POST | `/api/v1/setup/s3/test` | admin | HeadBucket + test PutObject |
| POST | `/api/v1/setup/s3/save` | admin | Save `external_s3`, complete wizard |
| POST | `/api/v1/setup/complete` | admin | Complete wizard without S3 |

While `!initial_setup_completed`, admin gets `403` `{ "error": "setup_required" }` except setup, login, `/me`, logout, health.

---

## 6. S3 design

Credentials in `SystemConfig.external_s3`; validated via AWS SDK v2; default Gateway connection created on save. Primary storage remains local FS (`STORAGE_DATA_DIR/objects/`).

---

## 7. Frontend

| Route | Purpose |
|-------|---------|
| `/setup` | Wizard for authed admin when `needs_setup` |
| `RequireSetupComplete` | Redirects to `/setup` until complete |

---

## 8. Documentation

- [User guide — Initial setup](../user-guide/README.md#first-time-setup)
- [Russian spec](../../ru/specs/initial-setup-wizard-tz.md)
