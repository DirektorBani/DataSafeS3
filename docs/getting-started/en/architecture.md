English | **[Русский](../ru/architecture.md)**

# Architecture overview

High-level architecture for operators. Deep technical reference: [../../en/context/architecture.md](../../en/context/architecture.md).

```mermaid
flowchart TB
  clients["Browser / AWS CLI / SDK"]
  caddy["Caddy :8080"]
  console["web/console"]
  server["storage-server :9000"]
  meta[(BoltDB or PostgreSQL)]
  fs["objects/ on disk"]
  clients --> caddy
  caddy --> console & server
  server --> meta & fs
```

## Request flows

### Console login (JWT)

```mermaid
sequenceDiagram
  participant U as User browser
  participant C as Caddy :8080
  participant S as storage-server
  participant M as Metadata DB
  U->>C: POST /api/v1/admin/login
  C->>S: forward
  S->>M: verify credentials
  S-->>U: JWT token
  U->>C: API calls Bearer JWT
```

### S3 operations (SigV4)

```mermaid
sequenceDiagram
  participant CLI as S3 client
  participant S as storage-server :9000
  participant FS as objects/
  CLI->>S: PutObject (signed)
  S->>S: verify SigV4
  S->>FS: write bytes
  S-->>CLI: 200 OK
```

## Data layout

| Path | Content |
|------|---------|
| `STORAGE_DATA_DIR/objects/` | Object bytes |
| `metadata.db` or PostgreSQL | Buckets, users, policies, tenants |

## Related

- [Gateway replication](../../en/user-guide/06-gateway-and-minio.md)
- [Database schema](../../en/database.md)
