# ADR 0004: Inventory jobs

**Status:** Accepted (partial — Admin CSV runtime in CE Evidence Pack)  
**Date:** 2026-07-10 (updated 2026-08-20)  
**Related:** A4 · Governance Evidence Pack v1.4  

## Context

Operators need capacity / listing exports for compliance and planning without full MSP billing.

## Decision

`internal/inventory.InventoryJob` + Admin API manual jobs writing CSV (download and optional dest-bucket). Works on **Bolt and Postgres**. In-process job registry (not durable across restart). Cap 100 000 objects/job.

Scheduled/cron leader-scoped worker and Parquet remain future work; AWS-compatible `GetBucketInventoryConfiguration` is explicitly out of scope for this slice.

## Stub → runtime

Package `internal/inventory` + `internal/api/inventory_handlers.go`. Ops checklist: `docs/use-cases/*/governance-evidence.md`.
