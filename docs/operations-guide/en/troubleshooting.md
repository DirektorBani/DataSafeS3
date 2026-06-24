English | **[Русский](../ru/troubleshooting.md)**

# Troubleshooting

## Common issues

| Symptom | Cause | Fix |
|---------|-------|-----|
| `setup_required` 403 | Setup wizard not completed | Finish `/setup` or `POST /setup/complete` |
| Docker build fails on Windows | WinHTTP proxy `127.0.0.1:10801` | `scripts\dev-docker-local-binary.cmd` |
| Console 404 on refresh | Caddy not serving SPA | `docker compose up -d caddy` |
| S3 403 SignatureDoesNotMatch | Wrong keys or clock skew | Check `STORAGE_ACCESS_KEY`, sync NTP |
| PostgreSQL connection refused | Profile not started | `docker compose --profile postgres up -d` |

## Logs

```bash
docker compose logs -f storage-server
```

Set `STORAGE_LOG_LEVEL=debug` for verbose output.

## Health checks

```bash
curl http://localhost:9000/api/v1/health
curl http://localhost:9000/metrics
```

## More

Legacy troubleshooting: [../../en/context/local-dev.md](../../en/context/local-dev.md) · [User guide §12](../../en/user-guide/README.md#12-troubleshooting)
