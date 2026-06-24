English | **[Русский](../ru/authentication.md)**

# API authentication

## Console JWT

```bash
curl -s -X POST http://localhost:9000/api/v1/admin/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"YOUR_PASSWORD"}'
```

Response: `{ "token": "eyJ..." }`

Use header: `Authorization: Bearer <token>`

## API tokens (Integration API)

1. Console → **Access → API tokens → Create**
2. Token prefix: `ds_...`
3. Use in Swagger UI or curl

## MFA login

If MFA enabled, login returns `mfa_required` and `mfa_token`. Complete with:

```http
POST /api/v1/mfa/login
{"mfa_token":"...","code":"123456"}
```

## S3 SigV4

Configure AWS CLI:

```ini
[profile datasafe]
aws_access_key_id = datasafe
aws_secret_access_key = datasafesecret
endpoint_url = http://localhost:9000
```

## OIDC

Browser SSO — no direct API key. Service accounts use API tokens or S3 keys.

Full details: [../../en/api/swagger.md](../../en/api/swagger.md)
