# datasafes3

Helm chart for a single DataSafeS3 node (Bolt, one PVC) or several nodes that share Postgres. The container image tag defaults to `v` plus `appVersion` (`v1.7.0` for this chart).

```bash
helm install ds deploy/helm/datasafes3 \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Production uses a Secret you create yourself. See `values-production.yaml`. HA uses `values-ha.yaml`: Postgres holds metadata, and Kafka or NATS are optional event sinks, not the cluster connection.

Guide: [docs/kubernetes.md](../../../docs/kubernetes.md).
