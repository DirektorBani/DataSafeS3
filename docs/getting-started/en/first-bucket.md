English | **[Русский](../ru/first-bucket.md)**

# Create your first bucket

## Via console

1. Sign in at http://localhost:8080
2. Complete [setup wizard](setup-wizard.md) if prompted
3. Go to **Buckets** → **Create bucket**
4. Enter a name (e.g. `documents`) and choose visibility (`private` or `public-read`)
5. Open the bucket → upload a file via drag-and-drop or **Upload**

![Object browser](../../images/screenshots/object-browser.png)

## Via Admin API

```bash
TOKEN=$(curl -s -X POST http://localhost:9000/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"YOUR_PASSWORD"}' | jq -r .token)

curl -X POST "http://localhost:9000/api/v1/buckets/my-bucket" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"visibility":"private"}'

curl -X PUT "http://localhost:9000/api/v1/buckets/my-bucket/objects/hello.txt" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: text/plain" \
  -d "Hello DataSafeS3"
```

## Via S3 CLI

```bash
aws --endpoint-url http://localhost:9000 s3 mb s3://my-bucket
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://my-bucket/
```

Default S3 credentials: `datasafe` / `datasafesecret` (change in production).

## Next

- [Onboarding](onboarding.md) — full first-day checklist
- [Dashboard and buckets](../../en/user-guide/02-dashboard-and-buckets.md)
