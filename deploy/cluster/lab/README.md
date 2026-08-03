# Cluster Docker lab

## Modes

| Mode | Command | Needs registry | What it proves |
|------|---------|----------------|----------------|
| Offline | `lab/up-local.sh` | No — local images only | etcd×3 + postgres + erasure storage + partial drills |
| **SSH Apply (release)** | `lab/up-ssh.sh` → `run-apply-ssh.sh` → `run-drills-ssh.sh` | Host reaches alpine CDN once (offline apk bundle); **no Docker apk HTTPS** | Live SSH Apply + Patroni promote ≤60s + unicast keepalived VIP — **0 SKIP** |

```bash
# Release gate (Docker VMs, no bare metal)
powershell -NoProfile -File scripts/cluster/lab/fetch-offline-bundle.ps1   # once
powershell -NoProfile -File scripts/cluster/lab/fetch-apk-closure.ps1     # once
docker build -f deploy/cluster/lab/Dockerfile.offline -t datasafe-cluster-node:lab deploy/cluster/lab
bash scripts/cluster/lab/up-ssh.sh
bash scripts/cluster/lab/run-apply-ssh.sh
bash scripts/cluster/lab/run-drills-ssh.sh
# expect: skip=0 fail=0

# Offline-only (VIP/Patroni SKIP expected)
bash scripts/cluster/lab/up-local.sh
bash scripts/cluster/lab/run-apply.sh
bash scripts/cluster/lab/run-drills.sh --lab
```

### SSH lab details

- 3 containers (`ds-lab-node0..2`) on `10.88.0.10–12`, SSH `:2221–2223`
- `cap_add: NET_ADMIN,NET_RAW` + `privileged` for keepalived
- VIPs: `10.88.0.100` (S3), `.101` (console), `.102` (postgres) via **unicast VRRP** (works on Docker bridge)
- Image: `datasafe-cluster-node:lab` from `Dockerfile.offline`

Honest limits: Docker Desktop is not bare-metal L2; unicast VRRP is the supported lab proof. Production still expects real VMs/subnet for keepalived.
