# ТЗ: HA и репликация v2 (distributed data plane + site replication)

**Версия:** 1.0  
**Дата:** 2026-07-04  
**Статус:** Спецификация для dev-агента  
**Целевой релиз:** v1.1.0 (CE), часть orchestration — EE optional  
**Аудитория:** Agent / maintainer  
**Связанные документы:** [reference-deployment-2node](../operations-guide/en/reference-deployment-2node.md), [scaling](../operations-guide/en/scaling.md), [gateway](../en/context/gateway.md), [08-federation](../en/user-guide/08-federation-and-cluster.md), QA: `_local/qa/ha-verification-retest-2026-07-03.md`

---

## 0. Контекст и проблема

### 0.1. Текущая реализация (as-is)

| Компонент | Реализация | Файлы / артефакты |
|-----------|------------|-------------------|
| Объекты | Локальный FS `STORAGE_DATA_DIR/buckets/...` | `internal/storage/fs.go` |
| Erasure | Lab XOR 2+1 / 4+2 codec, **не** в hot path PUT | `internal/storage/erasure/codec.go` |
| Metadata HA | Postgres streaming + read replica DSN | `docker-compose.ha.yml`, `STORAGE_POSTGRES_READ_REPLICA_DSN` |
| API «HA» | `storage-server-standby` + `STORAGE_READ_ONLY` | `internal/api/ha_handlers.go`, `docker-compose.ha.yml` |
| Failover | **Ручной** promote Postgres + restart | `scripts/postgres-failover.{ps1,sh}` |
| Cross-site | Gateway async queue → **external S3** | `internal/api/gateway_worker.go`, `internal/metadata/replication.go` |
| Federation | Read proxy GetObject/ListObjectsV2 | `internal/federation/proxy*.go` |
| Cluster UI | Flags `distributed_mode`, `erasure_coding_planned` | `internal/api/enterprise_handlers.go` |

### 0.2. Почему текущий подход ненадёжен

1. **Split metadata / data plane:** promote Postgres не восстанавливает объекты, если primary FS недоступен.
2. **Active-passive API на shared path** — industry anti-pattern для object storage; standby не даёт durability при потере диска primary.
3. **Gateway replication** — eventual DR, не intra-cluster HA; нет SLA lag, нет heal.
4. **Ручной failover** + migration race при dual-start (см. QA retest 2026-07-03).
5. **Доки vs продукт:** README «High availability» vs scaling «do not assume automatic failover».

### 0.3. Целевая модель (industry best practice)

```text
┌─────────────────────────────────────────────────────────────┐
│ Site A (primary deployment)                                  │
│  ┌──────────────┐   ┌─────────────────────────────────────┐ │
│  │ Metadata     │   │ Object plane: EC 4+2 (min), quorum   │ │
│  │ Postgres +   │   │ N drives / M nodes, self-heal        │ │
│  │ auto-failover│   │ (NOT shared FS + read-only standby)  │ │
│  └──────────────┘   └─────────────────────────────────────┘ │
│           │ async site replication (optional Site B)           │
└───────────┼─────────────────────────────────────────────────┘
            ▼
┌─────────────────────────────────────────────────────────────┐
│ Site B — DR / second site (bidirectional optional)           │
└─────────────────────────────────────────────────────────────┘
```

**Принципы:**

- **Data plane:** AP внутри site (erasure + quorum); не CP sync replication на WAN.
- **Metadata:** single-writer с **orchestrated** failover (не shell-only).
- **Cross-site:** async replication с метриками lag + automatic heal; Gateway к external S3 — **отдельный** BC tier.
- **Не расширять** `storage-server-standby` как основную HA-историю.

---

## 1. Цели

| # | Цель | Критерий успеха |
|---|------|-----------------|
| G1 | Durability внутри site | Выдержать падение **1 drive / 1 storage node** без потери объектов (EC 4+2) |
| G2 | Availability внутrie site | PUT/GET продолжаются при деградации одного shard-set member (quorum) |
| G3 | Orchestrated metadata HA | RTO failover metadata ≤ **5 min** в lab compose; documented RPO = replication lag |
| G4 | Site replication (CE) | Async репликация bucket/site DataSafeS3↔DataSafeS3 с lag metrics + heal |
| G5 | Честная документация | EN+RU: убрать misleading «active-passive HA»; RPO/RTO таблицы |
| G6 | Observability | Prometheus: lag, queue, degraded shards, site repl backlog |
| G7 | Обратная совместимость | Single-node FS mode остаётся default (`STORAGE_BACKEND=fs`) |

---

## 2. Non-goals (v1.1.0)

- Geo-dispersed erasure coding (EC across WAN) — только intra-site EC + inter-site async repl.
- Active-active multi-write **без** key partitioning / LWW policy (phase 4 backlog).
- S3 RTC-style **SLA contract** (15 min) — метрики yes, commercial SLA — EE backlog.
- Замена Postgres на CRDT metadata (Garage-style) — не в v1.1.
- K8s operator auto-failover для всего stack — только Helm hooks + documented Patroni sidecar pattern в v1.1.
- Удаление Gateway replication — остаётся для external S3 BC.

---

## 3. Архитектура v2

### 3.1. Storage backend abstraction

Ввести интерфейс поверх `FSBackend`:

```go
// internal/storage/backend.go (extend)
type Backend interface {
    PutObject(ctx, bucket, key, r, size, contentType) (etag, error)
    GetObject(ctx, bucket, key) (io.ReadCloser, ObjectMeta, error)
    DeleteObject(...)
    // multipart, versioning — delegate to existing FS paths initially or shard-aware paths
    Health(ctx) BackendHealth // degraded shards, heal jobs pending
}
```

Режимы (`STORAGE_OBJECT_BACKEND`):

| Value | Описание |
|-------|----------|
| `fs` | **Default.** Текущий `FSBackend` — без изменений поведения. |
| `erasure` | EC-backed backend; shards on `STORAGE_ERASURE_DATA_PATHS` (comma-separated dirs or `nodeId:path`). |

### 3.2. Erasure layout (production minimum)

| Profile | Layout | Fault tolerance | Overhead |
|---------|--------|-----------------|----------|
| `dev` | 2+1 XOR (existing codec) | 1 shard | 50% |
| `production` | **4+2** Reed-Solomon (implement; replace XOR for production profile) | 2 shards | 50% |

**Failure domain:** каждый shard path на отдельном volume; в compose — минимум 6 volumes или 2 nodes × 3 drives.

**Self-heal:** background worker `internal/storage/erasure/heal.go` — при `Health.degraded=true` rebuild missing shard from parity.

### 3.3. Metadata HA v2

| Компонент | v1.1 target |
|-----------|-------------|
| Postgres | Streaming replica + **Patroni** (compose profile `ha-patroni`) OR улучшенный `postgres-failover` с leader lock table |
| Leader lock | Таблица `ha_leader_lock` (migration `013_ha_leader`) — один active writer storage-server |
| Read scaling | `STORAGE_POSTGRES_READ_REPLICA_DSN` — без изменений |
| Failover | `scripts/ha/failover-metadata.ps1` — promote + update leader + rolling restart storage-server |

**Запрет:** два `storage-server` writer с одним DSN без leader lock (guard at startup).

### 3.4. Site replication (новое, CE)

Отдельно от Gateway (external S3):

| Aspect | Design |
|--------|--------|
| Scope | Replication **peer** = другой DataSafeS3 site (Admin registers like Federation) |
| Direction | `one-way` default; `bidirectional` opt-in with LWW + `STORAGE_SITE_REPL_KEY_PREFIX` guard |
| Transport | Async queue (reuse patterns from `gateway_worker.go` + `metadata/replication.go`) |
| Operations | PUT/DELETE replicate; initial bulk sync job |
| Heal | On peer recovery — backlog resync + `GET` proxy to peer while lagging (optional phase 3) |
| API | `POST /api/v1/site-replication/peers`, rules CRUD, `GET .../status` |

**Не путать:** Gateway = arbitrary S3 endpoint; Site replication = DataSafeS3↔DataSafeS3 contract with version header.

### 3.5. Deprecation current «HA standby API»

| Item | Action |
|------|--------|
| `storage-server-standby` in docs | Rename to **DR read replica (legacy)**; not recommended for new installs |
| `STORAGE_READ_ONLY` standby | Keep for DR drill; mark deprecated in OpenAPI/cluster status |
| `reference-deployment-2node.md` | Rewrite around EC + metadata failover + optional site repl |

---

## 4. Фазы реализации (agent execution order)

### Phase 1 — Erasure object backend (intra-site durability)

**Priority:** P0 · **Target:** v1.1.0 CE

#### 4.1.1. Tasks

1. Implement **Reed-Solomon 4+2** in `internal/storage/erasure/` (use `github.com/klauspost/reedsolomon` or stdlib-compatible RS — pin in go.mod; document license).
2. `ErasureBackend` implementing `storage.Backend`:
   - Shard naming: `{base}/{bucket}/{key}/shard-{i}.bin`
   - Metadata still in Postgres/Bolt (object row stores `backend=erasure`, `shard_set_id`, `size`, `etag`)
3. Migration `013_object_backend.sql` — columns on `objects` (nullable; fs rows unchanged).
4. Env:
   - `STORAGE_OBJECT_BACKEND=fs|erasure`
   - `STORAGE_ERASURE_LAYOUT=dev|production`
   - `STORAGE_ERASURE_DATA_PATHS=/data/disk1,/data/disk2,...`
5. Wire in `cmd/storage-server/main.go` backend selection.
6. Self-heal worker (interval env `STORAGE_ERASURE_HEAL_INTERVAL`, default 5m).
7. Metrics:
   - `datasafe_erasure_degraded_shard_sets`
   - `datasafe_erasure_heal_bytes_total`
8. Tests:
   - Unit: RS encode/decode, 2 shard loss recovery
   - Integration: PUT/GET with one path unmounted (simulate)

#### 4.1.2. Acceptance criteria

- [ ] Default `STORAGE_OBJECT_BACKEND=fs` — all existing tests green.
- [ ] With `erasure` + 6 paths, kill one path → GET still succeeds; heal restores shard.
- [ ] `go test ./internal/storage/...` + new integration tag `//go:build integration`.
- [ ] Compose profile `ha-erasure`: 6 volume mounts, single storage-server.
- [ ] EN/RU ops section «Erasure backend» in scaling.md.

---

### Phase 2 — Metadata orchestration + single writer

**Priority:** P0 · **Target:** v1.1.0 CE

#### 4.2.1. Tasks

1. Migration `014_ha_leader_lock.sql`:
   ```sql
   CREATE TABLE ha_leader_lock (
     lock_id TEXT PRIMARY KEY DEFAULT 'storage-server',
     holder_id TEXT NOT NULL,
     acquired_at TIMESTAMPTZ NOT NULL,
     expires_at TIMESTAMPTZ NOT NULL
   );
   ```
2. `internal/ha/leader.go` — acquire/renew lease (TTL 30s, renew 10s); refuse mutating API if not leader.
3. Startup: `STORAGE_NODE_ID` (uuid/host) required when `STORAGE_HA_ENABLED=true`.
4. Improve `scripts/postgres-failover.ps1`:
   - Idempotent promote
   - Release old leader lock
   - Print exact env updates
5. Fix compose race: **mandatory** `storage-server-standby` depends on primary healthy (already in `docker-compose.ha-local.yml`); document single-writer only.
6. `/healthz` extensions:
   ```json
   {
     "ha_enabled": true,
     "is_leader": true,
     "postgres_ok": true,
     "replication_lag_s": 0.001,
     "object_backend": "erasure",
     "erasure_degraded": false
   }
   ```
7. Console Cluster page: show leader, backend mode, degraded EC (replace placeholder flags).

#### 4.2.2. Acceptance criteria

- [ ] Two storage-server instances same DSN without HA → second exits or read-only with clear error.
- [ ] Failover script promotes postgres; new leader acquires lock; writes succeed ≤ 5 min in lab.
- [ ] `scripts/ha/test-ha-cluster.ps1` extended: leader check, erasure profile optional.
- [ ] QA checklist passes 8/8 + new leader steps.

---

### Phase 3 — Site replication (DataSafeS3 ↔ DataSafeS3)

**Priority:** P1 · **Target:** v1.1.0 CE (MVP one-way)

#### 4.3.1. Tasks

1. Package `internal/siterepl/`:
   - Peer registry (Postgres table `site_replication_peers`)
   - Rules (`site_replication_rules`: source bucket → peer + dest bucket)
   - Queue (`site_replication_tasks` — mirror gateway task schema)
   - Worker loop (reuse gateway batch size, retry, urlpolicy for peer endpoint)
2. Admin API (administrator only):
   - `GET/POST/DELETE /api/v1/site-replication/peers`
   - `GET/POST/DELETE /api/v1/site-replication/rules`
   - `POST /api/v1/site-replication/rules/{id}/sync`
   - `GET /api/v1/site-replication/status` — lag, pending, last error
3. Wire `OnObjectEvent` to enqueue **both** gateway (if rule) and site repl (if rule).
4. Metrics:
   - `datasafe_site_replication_lag_seconds`
   - `datasafe_site_replication_queue_depth` (extend existing replication metrics labels)
5. Console: new tab **Site replication** under Gateway OR sibling nav (administrator).
6. Federation: document difference; optional link peer registration.

#### 4.3.2. Acceptance criteria

- [ ] Two compose projects `datasafe-a` / `datasafe-b` on different ports — rule replicates bucket A→B within 60s (lab).
- [ ] Delete on source replicates delete (eventual).
- [ ] Status API shows pending=0 after sync.
- [ ] Feature-audit: add 3 checks (peer test, rule CRUD, object appears on peer).
- [ ] EN/RU admin guide + operations DR section updated.

---

### Phase 4 — Multi-node erasure + load balancing (stretch v1.1 or v1.2)

**Priority:** P2

- Multiple `storage-server` instances behind Caddy/K8s Service; all share erasure shard set via distinct `STORAGE_ERASURE_DATA_PATHS` **or** dedicated storage nodes.
- Read-only scale-out: nodes without leader lock serve GET/List only.
- Helm `values-ha.yaml` rewrite: remove `storageServerStandby`; add erasure paths + Patroni subchart reference.

---

## 5. Конфигурация (summary)

| Variable | Default | Phase |
|----------|---------|-------|
| `STORAGE_OBJECT_BACKEND` | `fs` | 1 |
| `STORAGE_ERASURE_LAYOUT` | `dev` | 1 |
| `STORAGE_ERASURE_DATA_PATHS` | — | 1 |
| `STORAGE_ERASURE_HEAL_INTERVAL` | `5m` | 1 |
| `STORAGE_HA_ENABLED` | `false` | 2 |
| `STORAGE_NODE_ID` | hostname | 2 |
| `STORAGE_HA_LEADER_TTL` | `30s` | 2 |
| `STORAGE_SITE_REPLICATION_ENABLED` | `false` | 3 |
| `STORAGE_SITE_REPL_WORKER_INTERVAL` | `2s` | 3 |

---

## 6. Миграция для операторов

### 6.1. Single-node FS → Erasure (same site)

1. Maintenance window; backup metadata + objects.
2. Deploy v1.1 with `STORAGE_OBJECT_BACKEND=erasure` and empty paths.
3. Run **background migrator** `scripts/ha/migrate-fs-to-erasure.ps1` (agent implements): read FS objects → rewrite via API or internal job.
4. Verify shard distribution + heal idle.

### 6.2. Legacy 2-node standby → v2

1. Do **not** deploy new standby read-only API for HA.
2. Adopt EC backend + postgres Patroni (or scripted failover).
3. Keep Gateway for off-site copy until site replication configured.

---

## 7. Тестирование (agent deliverables)

| Test | Location |
|------|----------|
| Erasure unit/integration | `internal/storage/erasure/*_test.go` |
| Leader lock race | `internal/ha/leader_test.go` |
| Site repl e2e | `internal/siterepl/worker_integration_test.go` |
| Compose HA | `scripts/ha/test-ha-cluster.ps1` (extend) |
| Erasure compose | `scripts/ha/test-erasure-backend.ps1` (new) |
| Site repl two-stack | `scripts/ha/test-site-replication.ps1` (new) |
| CI | Add job `ha-smoke` profile on PR (optional continue-on-error until stable) |

**Regression:** `go test ./...`, `scripts/feature-audit-test.ps1` — no regressions on default fs backend.

---

## 8. Документация (EN + RU, agent updates)

| Document | Change |
|----------|--------|
| `docs/operations-guide/en/scaling.md` | EC backend, site repl, deprecate standby HA |
| `docs/operations-guide/en/reference-deployment-2node.md` | Rewrite v2 topology |
| `docs/operations-guide/en/disaster-recovery.md` | RPO/RTO table |
| `docs/administrator-guide/en/replication.md` | Split Gateway vs Site replication |
| `docs/en/user-guide/08-federation-and-cluster.md` | Cluster page fields |
| `docs/en/context/architecture.md` | Data/metadata/site layers diagram |
| `README.md` | HA bullet — accurate wording |
| `CHANGELOG.md` | `[1.1.0]` section when shipped |
| OpenAPI full spec | New site-replication routes |

---

## 9. CE / EE граница

| Capability | Edition | Notes |
|------------|---------|-------|
| EC 4+2 backend | **CE** | Apache-2.0 |
| Leader lock + manual failover scripts | **CE** | |
| Site replication one-way | **CE** | |
| Bidirectional site repl + key prefix policies | **CE** v1.1 MVP; advanced conflict UI — EE backlog |
| Patroni helm subchart (automated failover) | **CE** docs + example; **EE** certified chart optional |
| Replication SLA dashboard / alerting pack | **EE** backlog |
| Active-active multi-site writes | **EE** phase 4 |

No license gates in code for Phase 1–3 CE features.

---

## 10. Риски и mitigations

| Risk | Mitigation |
|------|------------|
| RS performance on small objects | Min object size for EC; padding policy; benchmark in `docs/testing/performance-benchmarks.md` |
| Migrator downtime | Background migrator; dual-read during migration flag |
| Site repl conflict | One-way default; bidirectional requires explicit opt-in + prefix |
| Scope creep | Phase 4 explicitly out of v1.1 must-ship |

---

## 11. Definition of Done (v1.1.0 HA v2)

- [ ] Phases 1–3 acceptance criteria met (partial: lab scripts pass; production 4+2/multi-host and delete replication remain future hardening).
- [x] Default install unchanged (fs backend, HA off).
- [x] `scripts/ha/test-ha-cluster.ps1` and new scripts pass on Windows lab.
- [x] EN/RU docs aligned; no «automatic failover» claim without Patroni profile.
- [x] Prometheus metrics documented in monitoring.md.
- [x] CHANGELOG `[1.1.0]` complete.
- [x] Competitive honesty: project-status.md lists HA v2 shipped items.

---

## 12. Agent instructions (execution)

1. **Read first:** `internal/storage/fs.go`, `internal/storage/erasure/codec.go`, `internal/api/gateway_worker.go`, `internal/metadata/replication.go`, `docker-compose.ha.yml`, `_local/qa/ha-verification-retest-2026-07-03.md`.
2. **Implement Phase 1 → 2 → 3 sequentially**; do not start Phase 3 before Phase 1 backend interface stable.
3. **Do not** add `Co-authored-by: Cursor` to commits; follow `.cursor/rules/git-and-push.mdc`.
4. **Minimal diff:** keep `FSBackend` default; no breaking API changes for S3 clients.
5. **Commit strategy:** one commit per phase or logical PR-sized chunk with tests.
6. **Ask user** before `git push`.

---

## 13. Quick reference — файлы для создания/изменения

```
internal/storage/backend.go          # extend interface
internal/storage/erasure/rs.go       # NEW Reed-Solomon
internal/storage/erasure/backend.go  # NEW ErasureBackend
internal/storage/erasure/heal.go     # NEW heal worker
internal/ha/leader.go                # NEW leader lock
internal/siterepl/                   # NEW package (phase 3)
internal/metadata/postgres/migrations/013_*.sql
internal/metadata/postgres/migrations/014_*.sql
internal/api/site_replication_handlers.go
cmd/storage-server/main.go
scripts/ha/test-erasure-backend.ps1
scripts/ha/test-site-replication.ps1
scripts/ha/migrate-fs-to-erasure.ps1
docker-compose.ha-erasure.yml        # NEW profile
docs/operations-guide/en/scaling.md
docs/operations-guide/en/reference-deployment-2node.md
```

---

**Конец ТЗ v1.0**
