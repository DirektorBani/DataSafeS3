English | **[Русский](../../ru/user-guide/08-federation-i-cluster.md)**

# 8. Federation and Cluster

[← Monitoring](07-monitoring-and-databases.md) | [Table of contents](README.md)

> **Federation** and **Clusters** are **administrator-only** sections.

> **Single-node by default:** These features do **not** replace production multi-AZ HA by themselves. For HA patterns see [scaling](../../operations-guide/en/scaling.md) and [2-node reference](../../operations-guide/en/reference-deployment-2node.md).

---

## Clusters (trusted sites)

The **Clusters** page is your control center for **multi-site trust** and **replication between DataSafeS3 deployments**.

### What you can do today

| Capability | Status |
|------------|--------|
| View **local cluster** id and endpoint | **Implemented** |
| Generate **join token** (`dsjoin_*`) | **Implemented** — one-time, 15 min TTL |
| **Pair** with another site (automatic mTLS) | **Implemented** — no manual fingerprint step |
| List **trusted remote** clusters (status, cert expiry) | **Implemented** |
| **Revoke** a remote cluster | **Implemented** |
| **Replication rules** to remote buckets | **Implemented** (v1.1.0) |
| Local **HA nodes** (leader / standby) | **Implemented** when `STORAGE_HA_ENABLED=true` |

### Typical workflow — link two offices

1. Admin on **Office A** → **Clusters** → generate join token.
2. Admin on **Office B** → **Clusters** → enter A’s URL + token → connect.
3. On A → open remote cluster → add replication rule (`documents` → `documents-dr`).
4. Upload a file on A → verify on B.

Full guide: [Administrator — trusted clusters](../../administrator-guide/en/trusted-clusters.md) · [Operations — trusted clusters](../../operations-guide/en/trusted-clusters.md)

### Local HA vs trusted remote

| On the Clusters page | Meaning |
|----------------------|---------|
| **Local cluster** + HA node list | Multiple **nodes of this site** (leader lock, standbys) |
| **Trusted remote clusters** | A **separate DataSafeS3 installation** you paired with |

---

## Federation

**Federation** registers other DataSafeS3 servers for **read proxy** (GetObject + ListObjectsV2). It does **not** copy writes — use **Clusters** replication for that.

### What works today

| Capability | Status |
|------------|--------|
| Register remote endpoints | **Implemented** — Administration → **Federation** |
| Assign peer to a **cluster** (local or trusted remote) | **Implemented** |
| S3 **GetObject** via peer | **Implemented** |
| S3 **ListObjectsV2** prefix proxy | **Implemented** |
| Federation sync jobs | **Implemented** |
| Global automatic placement | **Planned** |

### How to register a peer

1. **Administration → Federation**.
2. **Register cluster**.
3. Fill **name**, **endpoint** (peer S3/API URL), **region** if needed.
4. Select **Cluster** — local site or a **trusted remote** you already paired under Clusters.
5. Save.

The table shows which cluster each federation entry belongs to.

---

## Comparison: Gateway vs Federation vs Clusters

| Feature | Purpose |
|---------|---------|
| **Gateway** | Copy to **external S3** (MinIO, AWS) |
| **Clusters** | **Trust** another DataSafeS3 + **replicate objects** (mTLS) |
| **Federation** | **Read** objects from another DataSafeS3 (registry + proxy) |
| **Site replication** | Legacy AK/SK replication (lab) |

---

## Useful links

- [Replication models](../../administrator-guide/en/replication.md)
- [Trusted clusters (operations)](../../operations-guide/en/trusted-clusters.md)
- [HA reference deployment](../../operations-guide/en/reference-deployment-2node.md)
- [Architecture](../context/architecture.md)
- [Roadmap](../context/roadmap.md)

---

[← Table of contents](README.md)
