# ADR 0003: Immutable backup golden path

**Status:** Accepted  
**Date:** 2026-07-10 (accepted 2026-07-14, v1.2.0)  
**Related:** A2 · TZ `D:\datasafe_tz\specs\v1.2.0\immutable-backup-and-hygiene-tz.md`

## Context

Ransomware / backup RFPs expect Object Lock + versioning as one operable path (restic, Kopia, Velero).

## Decision

Document and close AC gaps for versioning + Object Lock as a single **immutable backup** use-case:

- Use-case EN/RU: `docs/use-cases/*/immutable-backup.md`
- Admin/console: `retention_mode` (GOVERNANCE|COMPLIANCE) and versioning Suspend UI
- No multi-step console “wizard theater” in v1.2.0

Never claim WORM without Object Lock enabled. Bolt/Postgres parity for lock metadata is required for CE claims.

## Consequences

Operators get an honest golden path after MinIO migration. Full AWS Object Lock parity remains out of scope. HA promote / Kafka stay separate ADRs.
