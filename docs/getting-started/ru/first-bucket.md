**[English](../en/first-bucket.md)** | Русский

# Создание первого bucket

## Через консоль

1. Войдите на http://localhost:8080
2. Пройдите [мастер настройки](setup-wizard.md), если требуется
3. **Бакеты** → **Создать бакет**
4. Имя (например `documents`) и видимость (`private` или `public-read`)
5. Откройте bucket → загрузите файл drag-and-drop или **Загрузить**

![Браузер объектов](../../images/screenshots/object-browser.png)

## Через Admin API

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

## Через S3 CLI

```bash
aws --endpoint-url http://localhost:9000 s3 mb s3://my-bucket
aws --endpoint-url http://localhost:9000 s3 cp file.txt s3://my-bucket/
```

S3 по умолчанию: `datasafe` / `datasafesecret` (смените в production).

## Далее

- [Онбординг](onboarding.md) — полный чеклист первого дня
- [Главная и бакеты](../../ru/user-guide/02-dashbord-i-bakety.md)
