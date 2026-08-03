# Remote Apply scripts (Wave 2)

Run on Linux nodes via `cluster_push_apply.sh` / `Invoke-ClusterApplyWave2 -Apply`.

| Script | Role |
|--------|------|
| `install-packages.sh` | apt/dnf: etcd, Patroni, Postgres, HAProxy, keepalived, NFS |
| `apply-node.sh` | Idempotent-ish deploy of rendered bundle on one node |
| `health-gates.sh` | Best-effort HTTP/pg checks after Apply |

## Security

- Operator host uses `StrictHostKeyChecking=yes` and BatchMode for key auth.
- Password never on argv; `ssh_mode=P` Apply requires `--identity` after key bootstrap.
- Bundle with real secrets only for `--apply` / `-Apply`; DryRun stays redacted.
- NFS exports still node-IP only (from render security gate).

## Not in this slice

- Pulling/running `storage-server` container/binary on the leader (operator wires image + env after Patroni/NFS are up).
- Automated Patroni promote / VIP kill drills (manual lab AC).
