**[English](../en/curl-examples.md)** | Русский

# Примеры curl

Замените `TOKEN` на JWT после входа.

## Статус setup (публичный)

```bash
curl -s http://localhost:9000/api/v1/setup/status | jq
```

## Список buckets

```bash
curl -s http://localhost:9000/api/v1/buckets \
  -H "Authorization: Bearer $TOKEN" | jq
```

## Создать bucket

```bash
curl -s -X POST http://localhost:9000/api/v1/buckets/docs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"visibility":"private"}'
```

## Загрузить объект

```bash
curl -s -X PUT "http://localhost:9000/api/v1/buckets/docs/objects/report.pdf" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/pdf" \
  --data-binary @report.pdf
```

## Создать пользователя (admin)

```bash
curl -s -X POST http://localhost:9000/api/v1/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"User12345!","role":"user","email":"alice@corp.local"}'
```

## Создать tenant

```bash
curl -s -X POST http://localhost:9000/api/v1/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Corp"}'
```

## Журнал активности

```bash
curl -s "http://localhost:9000/api/v1/activity?limit=20" \
  -H "Authorization: Bearer $TOKEN" | jq
```

## S3 через AWS CLI

```bash
aws --endpoint-url http://localhost:9000 s3 ls
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://docs/
```

Все маршруты: [openapi-full.yaml](../../api/openapi-full.yaml).
