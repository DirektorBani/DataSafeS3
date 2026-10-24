# Kubernetes

English: [kubernetes.md](../kubernetes.md).

Чарт `datasafes3` ставит DataSafeS3. По умолчанию это один pod, PersistentVolumeClaim и метаданные Bolt. Больше ничего запускать не требуется.

Из репозитория Helm на GitVerse (`https://gitverse.ru/api/packages/DirektorBani/helm`):

```bash
helm repo add datasafes3 https://gitverse.ru/api/packages/DirektorBani/helm
helm repo update
helm install ds datasafes3/datasafes3 --version 1.7.0 \
  --set storageServer.image.repository=gitverse.ru/direktorbani/datasafe-storage-server \
  --set console.image.repository=gitverse.ru/direktorbani/datasafe-console \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Или из клона репозитория (чарт в нём уже по умолчанию берёт образы GitVerse):

```bash
helm install ds deploy/helm/datasafes3 \
  --set auth.accessKey=datasafe \
  --set auth.secretKey="$(openssl rand -hex 24)" \
  --set auth.adminPassword="$(openssl rand -hex 18)" \
  --set auth.jwtSecret="$(openssl rand -hex 24)" \
  --set auth.metricsToken="$(openssl rand -hex 16)"
```

Если `storageServer.image.tag` пуст, тег образа — `v` плюс `appVersion` чарта. Этот релиз: `gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0`. Реестр публичный. Те же образы есть в GHCR (`ghcr.io/direktorbani/...`) на зеркале GitHub.

`kubectl -n default port-forward svc/ds-datasafes3-s3 9000:9000` открывает S3 API. Сервис консоли — `ds-datasafes3-console`, порт 8080, когда `console.enabled` истинно. Пробы живости и готовности вызывают `GET /healthz`.

Интерфейс консоли доступен на русском и английском.

## Production

`values-production.yaml` не отрисуется, пока не задан `auth.existingSecret`. Сначала создайте Secret. Ключи: `accessKey`, `secretKey`, `adminUser`, `adminPassword`, `jwtSecret`, `metricsToken`.

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

Pod работает от UID 65532, корневая файловая система только для чтения, `/tmp` — пустой emptyDir. Объекты и Bolt лежат только на томе данных. В этом файле values включён TLS Ingress: создайте названный там TLS Secret и задайте имена хостов. Ingress консоли не отдаёт `/metrics`. Сервис S3 по-прежнему отдаёт `/metrics` на порту 9000 и закрывает его `metricsToken`, если ключ задан.

PodDisruptionBudget включён (`maxUnavailable: 1`). NetworkPolicy и ServiceMonitor Prometheus выключены, пока их не включат.

## Больше одного pod

`values-ha.yaml` включает `cluster.enabled` и Postgres рядом. Метаданные в Postgres. У каждого pod свой PVC с байтами объектов, процесс копирует их соседям. `STORAGE_HA_ENABLED` выбирает одного писателя. Kafka и NATS не устанавливаются. `events.kafkaBrokers` или `events.natsURL` задают, только если нужен такой приёмник.

Внешняя база: `postgres.enabled` оставьте false и задайте `postgres.external.host` плюс `postgres.external.existingSecret` (ключ `password`). При `cluster.enabled` отрисовка падает, если нет ни встроенной, ни внешней базы.

Это то же кластерное поведение, что уже есть у процесса. Это не автоматическое переключение между зонами доступности и не erasure между узлами.

## Проверки

```bash
bash scripts/helm-kind-smoke.sh
KIND_SMOKE=1 bash scripts/helm-kind-smoke.sh
```

Первая команда — `helm lint` и `helm template`. Вторая сама образ не собирает: загрузите `gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0` в локальный Docker и перетегируйте в `ghcr.io/direktorbani/datasafe-storage-server:v1.7.0` (под этим именем скрипт передаёт образ в kind), затем скрипт поднимает kind, ставит чарт и кладёт и читает объект через AWS CLI. В CI это делает образ из задания `docker-build`.

Прежний чарт с Caddy, Prometheus и Grafana: [helm-datasafe.md](helm-datasafe.md).
