English | **[Русский](../ru/curl-examples.md)**

# curl examples

Replace `TOKEN` with JWT from login.

## Setup status (public)

```bash
curl -s http://localhost:9000/api/v1/setup/status | jq
```

## List buckets

```bash
curl -s http://localhost:9000/api/v1/buckets \
  -H "Authorization: Bearer $TOKEN" | jq
```

## Create bucket

```bash
curl -s -X POST http://localhost:9000/api/v1/buckets/docs \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"visibility":"private"}'
```

## Upload object

```bash
curl -s -X PUT "http://localhost:9000/api/v1/buckets/docs/objects/report.pdf" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/pdf" \
  --data-binary @report.pdf
```

## Create user (admin)

```bash
curl -s -X POST http://localhost:9000/api/v1/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"User12345!","role":"user","email":"alice@corp.local"}'
```

## Create tenant

```bash
curl -s -X POST http://localhost:9000/api/v1/tenants \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"Acme Corp"}'
```

## Activity log

```bash
curl -s "http://localhost:9000/api/v1/activity?limit=20" \
  -H "Authorization: Bearer $TOKEN" | jq
```

## S3 with AWS CLI

```bash
aws --endpoint-url http://localhost:9000 s3 ls
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://docs/
```

See [openapi-full.yaml](../../api/openapi-full.yaml) for all routes.
