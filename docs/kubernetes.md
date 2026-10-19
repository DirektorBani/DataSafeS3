# Kubernetes

The chart `deploy/helm/datasafes3` installs DataSafeS3. The default is one pod, a PersistentVolumeClaim, and Bolt metadata. Nothing else has to be running.

```bash
helm install ds deploy/helm/datasafes3 \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

The image tag is `v` plus the chart `appVersion` when `storageServer.image.tag` is empty. This release is `ghcr.io/direktorbani/datasafe-storage-server:v1.7.0`.

`kubectl -n default port-forward svc/ds-datasafes3-s3 9000:9000` reaches the S3 API. The console Service is `ds-datasafes3-console` on port 8080 when `console.enabled` is true. Liveness and readiness both call `GET /healthz`.

## Production

`values-production.yaml` refuses to render unless `auth.existingSecret` is set. Create the Secret first. Keys are `accessKey`, `secretKey`, `adminUser`, `adminPassword`, `jwtSecret`, and `metricsToken`.

```bash
kubectl create secret generic datasafes3-auth \
  --from-literal=accessKey=... \
  --from-literal=secretKey=... \
  --from-literal=adminUser=admin \
  --from-literal=adminPassword=... \
  --from-literal=jwtSecret=... \
  --from-literal=metricsToken=...
helm install ds deploy/helm/datasafes3 -f deploy/helm/datasafes3/values-production.yaml
```

The pod runs as UID 65532 with a read-only root filesystem and a writable `/tmp` emptyDir. The data volume is the only place objects and Bolt metadata are stored. Ingress TLS is on in that values file; create the TLS Secret named there and set the host names. The console Ingress does not forward `/metrics`. The S3 Service still serves `/metrics` on port 9000, protected by `metricsToken` when that key is set.

A PodDisruptionBudget is on by default (`maxUnavailable: 1`). NetworkPolicy and a Prometheus ServiceMonitor are off until you enable them.

## More than one pod

`values-ha.yaml` sets `cluster.enabled` and a bundled Postgres. Metadata is in Postgres. Each storage pod keeps object bytes on its own PVC and copies them to peers. `STORAGE_HA_ENABLED` elects one writer. Kafka and NATS are not installed. Set `events.kafkaBrokers` or `events.natsURL` only when you want those sinks.

Use an external database by leaving `postgres.enabled` false and setting `postgres.external.host` plus `postgres.external.existingSecret` (key `password`). `cluster.enabled` fails the render when neither a bundled nor an external database is configured.

This is the same clustering the process already implements. It is not automatic failover across availability zones, and it is not erasure coding across nodes.

## Checks

```bash
bash scripts/helm-kind-smoke.sh
KIND_SMOKE=1 bash scripts/helm-kind-smoke.sh
```

The first command is `helm lint` and `helm template`. The second builds nothing by itself: load `ghcr.io/direktorbani/datasafe-storage-server:v1.7.0` into the local Docker, then the script creates a kind cluster, installs the chart, and puts and gets an object with the AWS CLI. CI does that with the image built in the `docker-build` job.
