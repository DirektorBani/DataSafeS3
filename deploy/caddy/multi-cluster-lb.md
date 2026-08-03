# Multi-cluster load balancer (write=leader, read=any)

Example Caddy front for a DataSafeS3 HA cluster. **Writes** (PUT/POST/DELETE and admin mutations) go to the current metadata leader; **reads** (GET/HEAD/LIST) may hit any healthy node.

## Requirements

- Each storage node exposes `GET /healthz` (liveness) and `GET /api/v1/ha/status` (leader flag when `STORAGE_HA_ENABLED=true`).
- Inter-cluster replication uses a separate port/path with **TLS passthrough** — do not terminate mTLS on this edge proxy.

## Caddyfile snippet

See [multi-cluster-lb.Caddyfile](./multi-cluster-lb.Caddyfile).

## Helm overlay

Enable the optional ConfigMap + Service in `deploy/helm/datasafe`:

```yaml
caddy:
  multiClusterLB:
    enabled: true
    writeUpstreams:
      - datasafe-storage-server-0:9000
      - datasafe-storage-server-1:9000
    readUpstreams:
      - datasafe-storage-server-0:9000
      - datasafe-storage-server-1:9000
```

Template: [deploy/helm/datasafe/templates/caddy-lb.yaml](../helm/datasafe/templates/caddy-lb.yaml).

## Compose overlay

Use with `deploy/compose/docker-compose.ha-local.yml`:

```powershell
# Leader + two followers behind Caddy (adjust upstreams to your node addresses)
docker compose -f deploy/compose/docker-compose.ha-local.yml -f deploy/caddy/docker-compose.lb-overlay.yml up -d
```

## Health checks

| Pool | Endpoint | Use |
|------|----------|-----|
| Write | `/api/v1/ha/status` | Route when `is_leader=true` |
| Read | `/healthz` | Any node with HTTP 200 |

## Notes

- S3 SigV4 uploads through a single write pool avoid split-brain metadata writes.
- Console `/api/v1/*` admin traffic should use the same write pool as S3 mutations.
- Trusted-cluster mTLS replication bypasses this LB; peers connect directly node-to-node.
