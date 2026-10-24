# Прежний чарт Helm datasafe

English: [deploy/helm/datasafe/README.md](../../deploy/helm/datasafe/README.md).

Это более ранний чарт. Он повторяет стек Compose: `storage-server`, Caddy, необязательные PostgreSQL, Prometheus и Grafana. Чарт, который описывает релиз v1.7.0, — [datasafes3](helm-datasafes3.md). Новый бой ставьте им. Этот чарт остаётся для уже развёрнутых кластеров, которым нужны Prometheus и Grafana внутри того же релиза Helm.

## Что нужно

- Kubernetes 1.24 и новее (у этого чарта своя планка; у `datasafes3` — 1.25)
- Helm 3.10 и новее
- StorageClass для данных и, если включён, для PostgreSQL
- Образы `gitverse.ru/direktorbani/datasafe-storage-server:v1.7.0` и `gitverse.ru/direktorbani/datasafe-console:v1.7.0` (реестр GitVerse публичный)

## Установка опубликованных тегов

Чарт ставится из клона репозитория: в репозиторий Helm на GitVerse публикуется только `datasafes3`. Образы по умолчанию берутся с GitVerse.

```bash
helm install datasafe deploy/helm/datasafe \
  --set storageServer.image.tag=v1.7.0 \
  --set console.image.tag=v1.7.0 \
  --namespace datasafe --create-namespace
```

Без Ingress:

```bash
kubectl port-forward -n datasafe svc/datasafe-caddy 8080:80
kubectl port-forward -n datasafe svc/datasafe-storage-server 9000:9000
```

Метаданные PostgreSQL: `--set postgres.enabled=true --set storageServer.metadataBackend=postgres`.

Файл `values-production.yaml` этого чарта задаёт заготовки секретов и `STORAGE_STRICT_SECRETS`. Необязательный Vault Agent: `deploy/helm/datasafe/examples/values-vault-agent.yaml`. Vault в установку по умолчанию не входит.

Обновление и снятие:

```bash
helm upgrade datasafe deploy/helm/datasafe -f my-values.yaml -n datasafe
helm uninstall datasafe -n datasafe
```

Интерфейс консоли доступен на русском и английском.
