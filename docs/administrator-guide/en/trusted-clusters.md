English | **[Русский](../ru/trusted-clusters.md)**

# Trusted clusters (console guide)

Step-by-step instructions for administrators using the web console. For architecture, env vars, and Docker lab setup, see [Operations guide — trusted clusters](../../operations-guide/en/trusted-clusters.md).

> **Who can use this:** system **administrator** only.

## Where to find it

| Menu item | Purpose |
|-----------|---------|
| **Clusters** | Local cluster, trusted remotes, pairing, replication rules, revoke |
| **Federation** | Register S3 read peers — each peer is tied to a **cluster** |
| **Site replication** | Classic DataSafeS3↔DataSafeS3 with access keys (legacy / lab) |

## Clusters page overview

After opening **Administration → Clusters** you see:

| Section | Content |
|---------|---------|
| **Local cluster** | Name, endpoint, cluster id, pairing controls |
| **Trusted remote clusters** | Sites you paired with — status, cert expiry, safety number (audit) |
| **Join another cluster** | Form to connect *this* site to an existing initiator |

Local HA nodes (leader, standbys) appear when `STORAGE_HA_ENABLED=true` — that is **intra-cluster** topology, not the same as a trusted remote site.

## Add a remote site (pairing)

You need admin access on **both** sites.

### On the initiator (Site A)

1. Open **Clusters**.
2. Click **Generate join token**.
3. Copy the `dsjoin_…` string immediately — it is **not** shown again.
4. Share it securely with the Site B admin (chat, password manager, QR if shown).

Token expires in **15 minutes** and works **once**.

### On the joiner (Site B)

1. Open **Clusters**.
2. Under **Join a cluster**, enter:
   - **Initiator URL** — API base Site A can receive pairing on (e.g. `https://ds-a.example.com` or lab `http://host.docker.internal:9002`).
   - **Join token** — the `dsjoin_…` string.
   - **Display name** (optional) — how Site A will label Site B.
3. Click **Connect & trust**.

If pairing succeeds, both consoles list the other site under trusted clusters with status **healthy**.

### If pairing fails

| Message / behaviour | What to check |
|---------------------|---------------|
| Token invalid / expired | Generate a new token on initiator |
| CA verification failed | Wrong initiator URL or TLS interception |
| Connection error | Firewall, `STORAGE_CLUSTER_ENDPOINT` not reachable from joiner container |
| 403 / not admin | Log in as administrator |

No manual “confirm certificate fingerprint” step is required — trust is established automatically via mTLS.

## Remote cluster detail

Click a trusted remote cluster to see:

| Field | Meaning |
|-------|---------|
| **Status** | `healthy`, `renewing`, or `revoked` |
| **Cert expires** | Client certificate expiry (auto-renewed ~75 days before) |
| **Next rotation** | When the rotator will renew |
| **Safety number** | Support/audit reference only |
| **Replication rules** | Bucket mappings to this remote |

### Add a replication rule

1. Create source bucket locally and destination bucket on the remote site (same or different name).
2. On remote cluster detail → **Add replication rule**.
3. Choose **source bucket** and **destination bucket**.
4. Upload a test file to the source — verify on remote after a few seconds.

Lag and queue depth: same as site replication — `GET /api/v1/site-replication/status` or cluster UI if shown.

### Revoke a remote cluster

**Revoke cluster** immediately stops replication and health checks to that peer on **your** site. Use when a site is decommissioned or compromised. Re-pairing requires a new join token.

## Federation and clusters

**Federation** registers another DataSafeS3 for **read proxy** (GetObject / ListObjectsV2). Since **v1.1.0**, each federation peer has a **Cluster** field:

| Cluster selection | When to use |
|-------------------|-------------|
| **Local cluster** | Peer serves reads for objects logically tied to this site |
| **Named remote trusted cluster** | Peer entry scoped to a site you already paired with |

Steps:

1. **Administration → Federation**.
2. **Register cluster** (peer).
3. Fill **name**, **endpoint**, **region** (if needed).
4. Select **Cluster** from the dropdown (local or trusted remote).
5. Save.

The table shows which cluster each federation peer belongs to.

Federation does **not** replace trusted-cluster replication for DR writes — it complements read scenarios.

## Comparison cheat sheet

| Need | Use |
|------|-----|
| Copy files to AWS/MinIO | **Gateway** |
| Copy files to another DataSafeS3 with mTLS trust | **Clusters** + replication rule |
| Copy with access keys only (lab) | **Site replication** |
| Read object from another DataSafeS3 without full repl | **Federation** |
| Multiple nodes, one site | **Cluster** page HA section + [scaling](../../operations-guide/en/scaling.md) |

## API (automation)

```http
GET  /api/v1/clusters
POST /api/v1/clusters/pairing-codes
POST /api/v1/clusters/pair/join
POST /api/v1/clusters/{id}/revoke
GET  /api/v1/clusters/{id}/replication-rules
POST /api/v1/clusters/{id}/replication-rules
DELETE /api/v1/clusters/{id}/replication-rules/{ruleId}
```

Full schema: [openapi-full.yaml](../../api/openapi-full.yaml).

## See also

- [Replication (all models)](replication.md)
- [Operations — trusted clusters](../../operations-guide/en/trusted-clusters.md)
- [User guide — federation & cluster](../../en/user-guide/08-federation-and-cluster.md)
