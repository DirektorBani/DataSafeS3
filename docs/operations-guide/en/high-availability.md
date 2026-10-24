English | **[Русский](../ru/high-availability.md)**

# High availability

Community Edition runs several storage-server processes against one Postgres database. Each process keeps its own object directory. After a write, the leader copies the stored bytes to the other nodes that have advertised an address. One process holds the leader lease. This is not automatic failover across availability zones, and it does not elect a Postgres primary. A filesystem mounted by every node is the legacy read-only standby; new installs use per-node disks or erasure shards ([2-node reference](reference-deployment-2node.md)).

The lab layout and ports are in the [2-node reference](reference-deployment-2node.md). Promote commands and the evidence pack are in [disaster recovery](disaster-recovery.md).

## Only the leader writes

With `STORAGE_HA_ENABLED=true` and the Postgres metadata backend, a follower rejects writes with **503** and body `not cluster leader` (`Retry-After: 30`). A follower proxies `GET` and `HEAD` of an object to the leader and keeps the original `Host`, so SigV4 still matches. If that dial fails, the follower serves the copy on its own disk. Bucket listings, `/healthz`, `/metrics`, and `/api/v1/health` are answered locally.

A local read whose metadata row is present and whose file is missing copies the stored bytes from a peer that has them, then opens that copy on this node. Public share download uses that path. The bytes copied are the bytes already stored, including ciphertext. When no peer can supply the file, that read is HTTP 503. The copy is accepted only when the size and stored ETag match the metadata row.

Peer copies use `PUT` and `GET /api/v1/ha/object-replica` on `STORAGE_INTERNAL_ADDR` (lab port 9002). When HA is enabled and the variable is unset, the server listens on `:9002`, and the `docker-compose.ha.yml` overlay sets it explicitly. Before 1.7.1 a standby replica answered 404 without it. The public listener and the load balancer do not serve that path. Each request is an HMAC of the method, path, body hash, expiry, nonce, and destination node id, using the shared JWT secret. A code lasts at most 30 seconds and cannot be replayed.

The same 503 applies to v1.6 sharing and access-security writes: global, tenant, and bucket sharing policy, creating and revoking share links, and presign. Public share GET and download stay on followers. See [Access model](access-model.md).

`GET /healthz` includes `ha_enabled`, `is_leader`, `node_id`, `version`, and `commit`.

The event dispatcher runs only on the leader. Followers update the pending gauge and leave rows alone.

## Takeover

A process that has never held the lease keeps trying. It takes the lease only after the previous one expires or is released, so it does not steal a live lease. Takeover time is about the lease that is still running: a 10 second lease was observed at about 9 seconds, and the default 30 second lease at about 26 seconds. `STORAGE_HA_LEADER_TTL` sets the lease. The renew interval is at most one third of that lease.

A process that already led and then failed to renew drops the lease immediately and does not acquire it again in the same process. That is what keeps a fenced old primary from writing. It is not a power fence.

When a node becomes leader, and about every 2 seconds, it reloads webhook and bucket-notification sinks from the database. A webhook created while the process was a follower is delivered after takeover. An outbox row whose sink is not registered yet stays pending. Those attempts are not counted toward dead.

## Promote and evidence

`scripts/ha/promote.sh` (and `promote.ps1`) fence the old Postgres primary (`default_transaction_read_only=on`), promote the standby, and on a fresh promote delete `ha_leader_lock` so the new storage node can take the lease. A second run leaves a live lease in place. Patroni labs use `scripts/cluster/promote.sh` with the same exit codes. Dry-run prints the plan and changes nothing.

`scripts/collect-dr-evidence.sh` writes a folder, `summary.md`, `evidence.json`, and a zip: timing, lag, healthz, an object write and read, outbox counts, dispatcher on the new leader, loss, and at-least-once duplicates. `bash scripts/ha/run-promote-drill.sh` is the one-command lab.

Nothing in this tree elects a new Postgres primary on its own. The leader copies object bytes to peers. The read-only standby lab still copies disks with `object-replicator` (`deploy/compose/docker-compose.ha-local.yml`).
