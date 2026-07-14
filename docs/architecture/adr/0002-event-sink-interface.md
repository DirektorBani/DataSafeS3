# ADR 0002: EventSink interface

**Status:** Proposed  
**Date:** 2026-07-10  
**Related:** A3 Kafka sink · TZ `D:\datasafe_tz\specs\v1.1.1\minio-migration-kit-tz.md` §6.3  

## Context

NATS notifications exist (`STORAGE_NATS_URL`). Platform teams also ask for Kafka. Multiple buses must not fork delivery logic.

## Decision (proposed)

Introduce `internal/events.EventSink` with `Name` / `Publish` / `Close`. Adapt existing NATS code to the interface when A3 lands; add Kafka behind `STORAGE_KAFKA_BROKERS`. **Do not** add Kafka to `go.mod` until A3 implementation.

## Stub

Interface file: `internal/events/sink.go` (v1.1.1). No runtime wiring yet.
