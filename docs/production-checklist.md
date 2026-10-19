# Production checklist

Use this before a single-node install is the copy of record. The one-node shape is one container, Bolt, and a volume. `STORAGE_DEV` stays false. A `.deb` or `.rpm` install is the same Bolt node: edit `/etc/datasafes3/datasafes3.env`, then `systemctl enable --now datasafes3`. Uninstall does not delete `/var/lib/datasafes3`. See [linux-packages.md](linux-packages.md).

## TLS

- Put Caddy, another reverse proxy, or the chart Ingress in front of the console and the S3 API.
- The production Compose profile `tls` reads `fullchain.pem` and `privkey.pem` from `deploy/compose/production/certs`.
- The Helm values file `values-production.yaml` turns Ingress TLS on. You create the certificate Secret.
- S3 clients need the proxy's certificate, or they will fail TLS verification. Path-style URLs stay on.

## Credentials

- Replace `STORAGE_ACCESS_KEY`, `STORAGE_SECRET_KEY`, `STORAGE_ADMIN_PASSWORD`, and `STORAGE_JWT_SECRET`. The process logs a warning when the historical defaults are still set, and `STORAGE_STRICT_SECRETS=true` refuses to start.
- On Kubernetes use `auth.existingSecret`. The chart does not ship those defaults, and `production: true` will not render without that Secret.
- Change the console password on first login and finish the setup wizard.
- Set `STORAGE_METRICS_TOKEN`. Without it, `/metrics` on the API port is open to anyone who can reach that port.

## What is exposed

- Do not publish port 9000 on a public interface if a proxy already terminates TLS. The production Compose file binds it to `127.0.0.1`.
- Do not put `/metrics` on the public proxy. The production Caddyfile does not forward it. Scrape `storage-server:9000/metrics` on the private network with the bearer token.
- `STORAGE_DEV=true` is a lab switch. It is off in the production Compose file and in the chart.

## Backups and restore

- The Bolt database is `metadata.db` inside `STORAGE_DATA_DIR`, next to the object files. Copy the whole data directory. A copy of the database without the files, or the files without the database, does not restore a bucket.
- Stop the process or snapshot the volume so the database and the files match. A file copy of a live Bolt database can tear a page.
- On Postgres metadata, dump the database and snapshot every node's object volume. Metadata alone does not contain the bytes.
- Write the admin password, the S3 secret, the JWT secret, and `STORAGE_SSE_MASTER_KEY` (if you set one) somewhere that is not the data volume. A new process with a different SSE key will not read objects sealed with the old key.
- Restore by putting the directory back and starting the same image version, or a newer one. Read [operations-guide/en/upgrade.md](operations-guide/en/upgrade.md) before skipping versions.

## Monitoring

- `GET /healthz` is the process health check. It returns JSON with `status` and `version`.
- Scrape `/metrics` with the bearer token. Example alerts live in `deploy/docker/prometheus/alerts.yml`.
- The chart can create a ServiceMonitor (`monitoring.serviceMonitor.enabled`). The token is the `metricsToken` key of the auth Secret.

## Upgrades

- Read the [1.7.0](../CHANGELOG.md) notes and the upgrade guide.
- Take the backup above before replacing the image.
- Pull the versioned tag (`v1.7.0`), not `latest`, for anything you need to reproduce.
- On more than one node, finish the rolling upgrade before enabling features the older binary does not understand. The 1.6.0 encryption gate is the pattern: a mixed cluster refuses the new feature until every node is current.

## Single-node still means single-node

The default install does not start Postgres, Kafka, or NATS. Adding those later is optional. Kafka and NATS receive events. They are not how pods share objects.
