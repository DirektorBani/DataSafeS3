# ADR 0003: Immutable backup golden path

**Status:** Proposed  
**Date:** 2026-07-10  
**Related:** A2 · TZ §6.2  

## Context

Ransomware / backup RFPs expect Object Lock + versioning as one operable path (restic, Kopia, Velero).

## Decision (proposed)

Document and close AC gaps for versioning + Object Lock as a single **immutable backup** use-case. Console wizard optional later. Never claim WORM without Object Lock enabled. Bolt/Postgres parity for lock metadata required for CE claims.

## Non-goals

Nextcloud-style collaboration; petabyte erasure as GA.
