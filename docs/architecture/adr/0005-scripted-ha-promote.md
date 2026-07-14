# ADR 0005: Scripted HA promote

**Status:** Proposed  
**Date:** 2026-07-10  
**Related:** A5 · TZ §6.5 · existing `scripts/ha/failover-metadata.ps1`  

## Context

v1.1.0 ships HA **lab** patterns. Market asks “what if primary dies?” without Patroni-class claims.

## Decision (proposed)

Prefer **scripts** (`scripts/ha/`) with lag checks, read-only flip, health wait. Package `internal/ha/promote` holds contracts only. CHANGELOG must not say automatic multi-AZ.

## Stub

`internal/ha/promote/doc.go` (v1.1.1).
