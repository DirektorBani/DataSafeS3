# ADR 0004: Inventory jobs

**Status:** Proposed  
**Date:** 2026-07-10  
**Related:** A4 · TZ §6.4  

## Context

Operators need capacity / listing exports for compliance and planning without full MSP billing.

## Decision (proposed)

`internal/inventory.InventoryJob` + leader-scoped worker (Postgres HA) writing CSV/Parquet to a dest bucket. Bolt: manual Admin export only unless parity is implemented.

## Stub

`internal/inventory/doc.go`, `types.go` (v1.1.1). No migrations yet.
