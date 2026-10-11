# ADR 0005: Scripted HA promote

**Status:** Accepted  
**Date:** 2026-07-10  
**Related:** A5 · TZ §E04 · `scripts/ha/promote.sh` · `scripts/cluster/promote.sh`  

## Context

v1.1.0 ships HA **lab** patterns. Market asks “what if primary dies?” without Patroni-class claims.

## Decision

Prefer **scripts** (`scripts/ha/`, `scripts/cluster/`) with lag checks, a read-only fence, and a health wait. Package `internal/ha/promote` holds the exit codes only. There is no in-process failover controller. CHANGELOG must not say automatic multi-AZ.

The old primary is fenced with `ALTER SYSTEM SET default_transaction_read_only = on` before `pg_promote`. That file is not replicated, so the promoted standby stays writable. A failed leader-lease renew marks that storage-server process not leader immediately. Patroni labs demote the old primary instead (it re-enters recovery).

## Stub

`internal/ha/promote/doc.go` (v1.1.1).
