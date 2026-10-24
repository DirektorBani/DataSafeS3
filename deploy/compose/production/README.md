# Production Compose

По-русски: [docs/ru/deploy-production.md](../../../docs/ru/deploy-production.md).



One storage container, Bolt metadata, a Caddy edge, and a named volume. `STORAGE_DEV` is `false` in the compose file. Caddy does not forward `/metrics`. The S3 port is bound to `127.0.0.1` only.

```bash
cp deploy/compose/production/.env.example deploy/compose/production/.env
# edit the secrets
docker compose --env-file deploy/compose/production/.env \
  -f deploy/compose/production/docker-compose.yml up -d
```

Open `http://127.0.0.1:8080`, change the admin password, and finish setup. S3 is `http://127.0.0.1:9000` on the host.

TLS: place `fullchain.pem` and `privkey.pem` in `deploy/compose/production/certs` and add `--profile tls`.

Backups and the rest of the go-live list: [docs/production-checklist.md](../../../docs/production-checklist.md).
