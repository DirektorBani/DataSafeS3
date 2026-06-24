**[English](../en/authentication.md)** | Русский

# Аутентификация API

## JWT консоли

```bash
curl -s -X POST http://localhost:9000/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"YOUR_PASSWORD"}'
```

Ответ: `{ "token": "eyJ..." }`

Заголовок: `Authorization: Bearer <token>`

## Токены API (Integration API)

1. Консоль → **Доступ → Токены API → Создать**
2. Префикс токена: `ds_...`
3. Использование в Swagger UI или curl

## Вход с MFA

При включённом MFA login возвращает `mfa_required` и `mfa_token`. Завершение:

```http
POST /api/v1/mfa/login
{"mfa_token":"...","code":"123456"}
```

## S3 SigV4

Настройка AWS CLI:

```ini
[profile datasafe]
aws_access_key_id = datasafe
aws_secret_access_key = datasafesecret
endpoint_url = http://localhost:9000
```

## OIDC

SSO через браузер — без прямого API key. Сервисные аккаунты: API tokens или S3 keys.

Подробнее: [../../ru/api/swagger.md](../../ru/api/swagger.md)
