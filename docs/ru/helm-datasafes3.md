# Чарт Helm datasafes3

English: [deploy/helm/datasafes3/README.md](../../deploy/helm/datasafes3/README.md).

Чарт одного узла DataSafeS3 (Bolt, один PVC) или нескольких узлов с общими метаданными PostgreSQL. Тег образа по умолчанию — `v` плюс `appVersion` (`v1.7.0` у этого чарта).

```bash
helm install ds deploy/helm/datasafes3 \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Production использует Secret, который создаёте вы. См. `values-production.yaml`. HA — `values-ha.yaml`: метаданные в PostgreSQL, Kafka и NATS остаются необязательными приёмниками событий, а не связью кластера.

Полные шаги: [kubernetes.md](kubernetes.md). Зеркало GitVerse: [gitverse-mirror.md](gitverse-mirror.md).

Интерфейс консоли доступен на русском и английском.

Запросы пода: сервер 100m CPU и 256 МиБ (предел 1 CPU и 1 ГиБ), консоль 50m CPU и 64 МиБ (предел 500m и 256 МиБ). PVC по умолчанию 20 ГиБ. `kubeVersion` чарта: Kubernetes 1.25 и новее.
