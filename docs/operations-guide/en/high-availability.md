English | **[Русский](../ru/high-availability.md)**

# High availability

Community Edition runs several storage-server processes against one Postgres database and a shared object volume. One process holds the leader lease. This is not automatic failover across availability zones, and it does not elect a Postgres primary.

The lab layout and ports are in the [2-node reference](reference-deployment-2node.md). Promote commands and the evidence pack are in [disaster recovery](disaster-recovery.md).

## Only the leader writes

With `STORAGE_HA_ENABLED=true` and the Postgres metadata backend, a follower rejects writes with **503** and body `not cluster leader` (`Retry-After: 30`). `GET`, `HEAD`, and `OPTIONS` still succeed, as do `/healthz`, `/metrics`, and `/api/v1/health`.

`GET /healthz` includes `ha_enabled`, `is_leader`, `node_id`, `version`, and `commit`.

The event dispatcher runs only on the leader. Followers update the pending gauge and leave rows alone.

## Takeover

A process that has never held the lease keeps trying. It takes the lease only after the previous one expires or is released, so it does not steal a live lease. Takeover time is about the lease that is still running: a 10 second lease was observed at about 9 seconds, and the default 30 second lease at about 26 seconds. `STORAGE_HA_LEADER_TTL` sets the lease. The renew interval is at most one third of that lease.

A process that already led and then failed to renew drops the lease immediately and does not acquire it again in the same process. That is what keeps a fenced old primary from writing. It is not a power fence.

When a node becomes leader, and about every 2 seconds, it reloads webhook and bucket-notification sinks from the database. A webhook created while the process was a follower is delivered after takeover. An outbox row whose sink is not registered yet stays pending. Those attempts are not counted toward dead.

## Promote and evidence

`scripts/ha/promote.sh` (and `promote.ps1`) fence the old Postgres primary (`default_transaction_read_only=on`), promote the standby, and on a fresh promote delete `ha_leader_lock` so the new storage node can take the lease. A second run leaves a live lease in place. Patroni labs use `scripts/cluster/promote.sh` with the same exit codes. Dry-run prints the plan and changes nothing.

`scripts/collect-dr-evidence.sh` writes a folder, `summary.md`, `evidence.json`, and a zip: timing, lag, healthz, an object write and read, outbox counts, dispatcher on the new leader, loss, and at-least-once duplicates. `bash scripts/ha/run-promote-drill.sh` is the one-command lab.

Nothing in this tree elects a new Postgres primary on its own. Object bytes stay on the node that wrote them unless you replicate them separately.
