# Rendered by Vault Agent — sourced by storage-server entrypoint wrapper.
{{- with secret "secret/data/datasafe/bootstrap" -}}
STORAGE_JWT_SECRET={{ .Data.data.jwt_secret }}
STORAGE_SECRET_KEY={{ .Data.data.s3_secret_key }}
STORAGE_ADMIN_PASSWORD={{ .Data.data.admin_password }}
STORAGE_MFA_ENCRYPTION_KEY={{ .Data.data.mfa_encryption_key }}
STORAGE_SSE_MASTER_KEY={{ .Data.data.sse_master_key }}
STORAGE_POSTGRES_PASSWORD={{ .Data.data.postgres_password }}
STORAGE_POSTGRES_DSN={{ .Data.data.postgres_dsn }}
STORAGE_OIDC_CLIENT_SECRET={{ .Data.data.oidc_client_secret }}
STORAGE_LDAP_BIND_PASSWORD={{ .Data.data.ldap_bind_password }}
{{- end }}
STORAGE_DEV=false
STORAGE_STRICT_SECRETS=true
STORAGE_ADMIN_USER=admin
