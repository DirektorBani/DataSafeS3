English | **[Русский](../../ru/specs/file-collaboration-status.md)**

# File collaboration — implementation status

**Last updated:** 2026-06-19  
**Spec:** [file-collaboration-tz.md](./file-collaboration-tz.md)  
**Docs baseline:** `24c2cbe` · **Code:** file collaboration Phases 1–3 in working tree (pending code commit)  
**Classification:** Product / engineering reference

---

## Summary

| Phase | Scope | Status |
|-------|-------|--------|
| **1** | Web «My files», home bucket, owner grants, Share UI | **Implemented** |
| **2** | Prefix grants, in-app notifications, home bucket quota | **Implemented** |
| **3** | Desktop folder sync (`datasafe-sync` CLI + Tauri shell) | **Implemented** |
| **4** | Mobile (Flutter) + mobile-web PWA | **Backlog** — see [phase4-backlog](./file-collaboration-phase4-backlog.md) |

Honest positioning: DataSafeS3 supports **personal web workspace + bucket/folder sharing + desktop folder sync** (`datasafe-sync` CLI and optional Tauri UI). Mobile clients are **backlog** (prototype code only). No OS file-provider integration or real-time co-editing.

---

## Phase 1 — Web workspace (done)

### Backend

| Capability | Status | Location |
|------------|--------|----------|
| Auto home bucket on first login | Done | `internal/api/home_bucket.go`, env `STORAGE_AUTO_HOME_BUCKET`, `STORAGE_HOME_BUCKET_NAME` |
| Home bucket default quota | Done | `STORAGE_HOME_BUCKET_MAX_SIZE_BYTES` (default 10 GiB), `STORAGE_HOME_BUCKET_MAX_OBJECTS` |
| `GET /api/v1/buckets` enrichment | Done | `access.ownership` (`owned` \| `shared` \| `tenant`), `can_read`, `can_write`, `shared_by` — `bucket_access_service.go` |
| Bucket list filter | Done | `?filter=owned\|shared\|tenant\|all` |
| Owner grant API | Done | `GET\|PUT\|DELETE /api/v1/buckets/{bucket}/access` |
| Shareable user picker | Done | `GET /api/v1/shareable-users?bucket=&q=` |
| Tenant admin grants (unchanged) | Done | `/api/v1/tenants/{tenant}/buckets/{bucket}/access` |
| Grantee RBAC on S3/JSON | Done | `grantBucketKeysForUser`, `canManageBucketGrants` |
| Tests | Done | `internal/api/home_bucket_test.go`, bucket access tests |

### Console

| Capability | Status | Location |
|------------|--------|----------|
| Nav «Files» for role `user` | Done | `sidebar.tsx`, `nav:files` |
| Tabs My files / Shared with me / Team | Done | `buckets.tsx` |
| Share tab (owner + tenant_admin) | Done | `bucket-detail.tsx` |
| Shareable users picker | Done | `api.listShareableUsers` |
| Empty state for home bucket | Done | `buckets.tsx` |

### Configuration (`.env.example`)

```env
STORAGE_AUTO_HOME_BUCKET=true
STORAGE_HOME_BUCKET_NAME=files
STORAGE_HOME_BUCKET_MAX_SIZE_BYTES=10737418240
```

### User documentation

- [User guide — Files and sharing](../user-guide/README.md#files-and-sharing)
- [Corporate file storage use case](../../use-cases/en/corporate-file-storage.md)

---

## Phase 2 — Prefix sharing & notifications (done)

### Data model (PostgreSQL migration `010`)

| Table | Purpose |
|-------|---------|
| `bucket_prefix_access_grants` | Folder-level read/write per user |
| `user_notifications` | In-app «shared with you» events |
| `recent_items` | Recent bucket/prefix access (foundation) |

BoltDB: parallel stores in `internal/metadata/collaboration_phase2.go`, `prefix_grants.go`.

### API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/v1/notifications` | List notifications + unread count |
| POST | `/api/v1/notifications/{id}/read` | Mark read |
| POST | `/api/v1/notifications/read-all` | Mark all read |
| GET | `/api/v1/recent` | Recent buckets/prefixes |
| PUT | `/api/v1/buckets/{bucket}/access` | Body includes `prefix_grants[]` alongside `grants[]` |

Bucket list: `access.shared_prefixes[]` for folder-only grantees.  
Prefix-aware list access: `allowedListPrefix` (JSON API), `EffectiveListPrefixForAccessKey` (S3).

### Console

| Capability | Status |
|------------|--------|
| Prefix grants in Share tab | Done |
| Notification bell | Done — `notification-bell.tsx`, «Mark all read» |
| Shared tab shows folder prefixes | Done — `access.shared_prefixes` in `buckets.tsx` |

---

## Phase 3 — Desktop sync (implemented)

| Component | Status | Path |
|-----------|--------|------|
| Sync engine | Done — pull/push, delete sync, conflicts, fsnotify watch | `internal/syncapp/` |
| CLI | Done — login, sync, watch, status, buckets, conflicts, resolve, token set, JSON | `cmd/datasafe-sync/` |
| Tauri desktop | Done — tray, folder picker, watch, bucket list, conflict panel | `clients/desktop/` |
| Conflict handling | Done — policies: last_write_wins, local/remote wins, keep_both (`.datasafe-conflicts/`) | `syncapp/conflict.go` |
| Sidecar build | Done — `scripts/build-datasafe-sync.ps1` / `.sh` | `clients/desktop/src-tauri/binaries/` |
| Code signing / auto-update | Not shipped | Future ops |

Quick start: [clients/README.md](../../../clients/README.md) · [desktop/README.md](../../../clients/desktop/README.md)

---

## Phase 4 — Mobile (backlog)

Prototype code exists (`clients/mobile`, `clients/mobile-web`) but **not** part of current delivery. See [file-collaboration-phase4-backlog.md](./file-collaboration-phase4-backlog.md).

| Component | Status | Path |
|-----------|--------|------|
| Flutter app | Backlog | `clients/mobile/` |
| Mobile web PWA | Backlog | `clients/mobile-web/` |

---

## What is still not available

| Capability | Notes |
|------------|-------|
| Native OS sync (Finder / Explorer integration) | Requires Phase 3 hardening or third-party tools |
| Mobile background sync | Phase 4+ |
| Real-time co-editing | Out of product scope |
| Email/push notifications | In-app only today |
| Full audit of every folder share view | Activity log on grant changes; not per-file read audit |

---

## Verification (2026-06-19)

| Check | Result |
|-------|--------|
| `go test ./...` | **PASS** |
| `home_bucket_test.go` | PASS (prefix shared, `shared_prefixes`) |
| `syncapp` / `sync_client_test` | PASS |
| feature-audit 93/93 | **PASS** |
| Console `npm run build` | **PASS** |
| OpenAPI | `/buckets` access fields, `/shareable-users`, `/notifications`, `/recent`, `/notifications/read-all` |

### Bug fixes (2026-06-23)

- S3/API list: prefix-only grantees no longer see full bucket listings
- `ownership=shared` for prefix-only grants in bucket list filter
- Postgres `recent_items` id includes `user_id` (no cross-user collision)
- DELETE bucket access revokes prefix grants for user
- Access PUT validates before replace; notifications only for new grantees
- Recent links `?prefix=` honored in bucket detail
- Tauri desktop: login before sync
- S3 ListObjects/ListObjectVersions: prefix clamp for folder-only grantees (`EffectiveListPrefixForAccessKey`)
- `POST /notifications/read-all` + console «Mark all read»
- `access.shared_prefixes[]` on bucket list for folder shares
- Migration `011_recent_items_user_id` — purge stale recent-item ids
- `datasafe-sync`: save folder/bucket/prefix to profile on sync/watch

---

## Related documents

- [file-collaboration-tz.md](./file-collaboration-tz.md) — original Phase 1 spec
- [competitive-assessment-2026-v5.md](../../analysis/competitive-assessment-2026-v5.md) — market score **9.1/10**
- [tenant-bucket-isolation-tz.md](./tenant-bucket-isolation-tz.md) — grants foundation
