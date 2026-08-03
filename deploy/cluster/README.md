# Cluster packaging

Internal design: `D:\datasafe_tz\specs\v1.2.0\cluster-installer-erasure-multilb-tz.md`

| Wave | Contents |
|------|----------|
| **1** | Installer menu, inventory, SSH [P]/[K], preflight |
| **2** | Render + SSH Apply + storage-server on leader |
| **Lab** | `deploy/cluster/lab` — offline compose or SSH nodes |
| **3** | Erasure C2 network shards, S3 any-node LB |

```bash
bash scripts/cluster/lab/up.sh
bash scripts/cluster/lab/run-apply.sh
bash scripts/cluster/lab/run-drills.sh --lab
```

Templates under `templates/`. Remote scripts under `../scripts/cluster/remote/`.
Do **not** commit secrets or lab private keys.
