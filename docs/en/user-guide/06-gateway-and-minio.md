English | **[Русский](../../ru/user-guide/06-gateway-i-minio.md)**

# 6. Gateway replication

[← Administration](05-administration.md) | [Table of contents](README.md) | Next: [Monitoring →](07-monitoring-and-databases.md)

> The **Gateway** section is available only to the **administrator**.

---

## Why Gateway?

**Gateway** automatically **copies files** from DataSafeS3 to **external S3-compatible storage**:

- off-site backup buckets;
- a second region or datacenter;
- any endpoint that speaks the S3 API.

Copying runs **in the background** — shortly after upload, the object appears on the remote side.

![Gateway — Connections tab](../../user-guide/images/gateway.png)

---

## Preparation: local S3 test endpoint

For lab testing, run a separate S3-compatible container on ports **9100** (API) and **9101** (web UI) so it does not conflict with DataSafeS3 **9000**:

```cmd
docker run -d --name datasafe-minio-test -p 9100:9000 -p 9101:9001 -e MINIO_ROOT_USER=s3test -e MINIO_ROOT_PASSWORD=s3testsecret minio/minio server /data --console-address ":9001"
```

| Service | Address | Credentials (example) |
|---------|---------|------------------------|
| Remote S3 API | http://localhost:9100 | `s3test` / `s3testsecret` |
| Remote web UI | http://localhost:9101 | same |

> The container name `datasafe-minio-test` is used by project scripts; the image is a standard S3-compatible server for local labs only.

---

## Auto-setup (script)

After DataSafeS3 and the test endpoint are running:

```cmd
scripts\setup-minio-gateway.cmd
```

The script (idempotent):

1. Creates bucket `replica-test` on the remote endpoint.
2. Adds connection **External S3 Test** in Gateway.
3. Tests the connection.
4. Creates a replication rule (local bucket → `replica-test`).

Optional source bucket:

```cmd
set GATEWAY_SOURCE_BUCKET=my-data
scripts\setup-minio-gateway.cmd
```

---

## Manual setup in the console

### Step 1 — Connection

1. **Gateway** → **Connections** → **Add Connection**.
2. Example for the local test endpoint:

| Field | Example |
|-------|---------|
| Name | `External S3 Test` |
| Endpoint | `http://host.docker.internal:9100` *(from DataSafeS3 Docker)* or `http://localhost:9100` |
| Region | `us-east-1` |
| Access Key | `s3test` |
| Secret Key | `s3testsecret` |
| Path-style | ✓ enable |
| Verify TLS | disable for HTTP |

3. **Test Connection** — should show connected.

### Step 2 — Replication rule

1. **Replication Rules** tab.
2. **Source bucket** — your DataSafeS3 bucket (e.g. `my-data`).
3. **Remote connection** — `External S3 Test`.
4. **Remote bucket** — `replica-test`.
5. **Add Rule**.

### Step 3 — Verification

1. Upload a file to the local bucket.
2. **Sync Jobs** / **Health** — check queue and bytes replicated.
3. Open the remote web UI on http://localhost:9101 and verify bucket `replica-test`.

---

## Gateway tabs

| Tab | Purpose |
|-----|---------|
| **Connections** | External S3 endpoints |
| **Replication Rules** | Local bucket → remote bucket mapping |
| **Sync Jobs** | Queue and history |
| **Health** | Errors, throughput, pending tasks |

---

## Common issues

| Problem | Solution |
|---------|----------|
| Test Connection failed | Check endpoint, path-style, credentials |
| Object not on remote side | **Health** → errors; is the queue decreasing? |
| Connection refused from Docker | Use `http://host.docker.internal:9100` on Windows/Mac |
| Cannot delete connection | Remove replication rules that reference it first |

More: [docs/context/gateway.md](../context/gateway.md)

---

## What's next?

- [Grafana and databases →](07-monitoring-and-databases.md)
