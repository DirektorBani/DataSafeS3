English | **[Русский](../../ru/user-guide/08-federation-i-cluster.md)**

# 8. Federation and Cluster

[← Monitoring](07-monitoring-and-databases.md) | [Table of contents](README.md)

> The **Federation** and **Cluster** sections are available only to the **administrator**.

> **Single-node by default:** Federation and Cluster do **not** provide multi-AZ high availability today. Federation adds **cross-site S3 read proxy** (GetObject + ListObjectsV2) when peers are registered — see below. For HA patterns see [scaling](../../operations-guide/en/scaling.md) and [2-node reference](../../operations-guide/en/reference-deployment-2node.md).

---

## Federation

**Federation** is a **registry of other DataSafeS3 servers** plus an **S3 proxy** for reads across registered peers.

### What works today

| Capability | Status |
|------------|--------|
| Register remote DataSafeS3 endpoints | **Implemented** — Administration → Federation |
| S3 **GetObject** via peer | **Implemented** — when object not found locally |
| S3 **ListObjectsV2** prefix proxy | **Implemented** — lists objects on remote peer for a prefix |
| Background sync worker | **Implemented** — federation sync jobs in console |
| Automatic data placement / global search | **Planned** |

### Why register a remote cluster

- track branch offices or backup sites;
- unified endpoint registry;
- foundation for future cross-site search (MVP for now).

### How to Add

1. **Administration → Federation**.
2. **Register cluster**.
3. Specify:
   - **name** (for example "Moscow Office");
   - **endpoint** (S3/API URL of the other DataSafeS3);
   - **region** (for example `us-east-1`).
4. Save.

> Today this is a **configuration registry** — automatic replication between two DataSafeS3 instances via Federation differs from Gateway (Gateway copies to **any** S3, including external S3).

---

## Cluster

**Cluster** shows whether DataSafeS3 runs as a **single server** or in **distributed mode** (multiple nodes).

### What Appears on the Cluster Page

| Field | Description |
|-------|-------------|
| Distributed mode | Whether distributed mode is enabled |
| Erasure coding | Whether erasure coding is planned (flag for now; algorithm on roadmap) |
| Disk paths | Paths to disks (for future multi-disk) |
| Nodes | List of nodes and their status |

### Single-Node Mode (Default)

For most installations, **one server** is enough — this is the main production path:

- one `local` node @ `localhost:9000`, status `healthy`;
- all data on one host.

### Distributed Mode (MVP)

In **Settings → Cluster**, an administrator can:

- enable **Distributed mode**;
- specify **disk paths** (one path per line);
- check **Erasure coding planned**.

> A full HA cluster with erasure coding is **still in development**. Today this is configuration and monitoring for the future, not automatic data distribution.

### Configuration via API

For advanced scenarios, cluster parameters can be set through the REST API `PUT /api/v1/settings/system` — see [architecture.md](../context/architecture.md).

---

## Comparison: Gateway vs Federation vs Cluster

| Feature | Purpose |
|---------|---------|
| **Gateway** | Copy files to **external S3** — **works today** |
| **Federation** | Registry + **GetObject / ListObjectsV2 proxy** to other DataSafeS3 sites — MVP |
| **Cluster** | Multiple **nodes of one** DataSafeS3 — foundation + UI |

---

## Useful Links

- [Guide: Gateway](06-gateway-and-minio.md)
- [Technical architecture](../context/architecture.md)
- [Roadmap](../context/roadmap.md)
- [Project status](../context/project-status.md)

---

[← Table of contents](README.md)
