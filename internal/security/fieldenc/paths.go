package fieldenc

// Canonical field paths for AAD and store hooks (docs/specs/field-encryption-1.0.3-tz.md).
const (
	PathAccessKeySecretKey    = "access_keys.secret_key"
	PathAccessKeySessionToken = "access_keys.session_token"
	PathGatewayAccessKey      = "gateway_connections.access_key"
	PathGatewaySecretKey      = "gateway_connections.secret_key"
	PathLDAPBindPassword      = "system_config.ldap.bind_password"
	PathOIDCClientSecret      = "system_config.oidc.client_secret"
	PathExternalS3Secret      = "system_config.external_s3.secret_access_key"
	PathLoggingESPassword     = "system_config.logging.elasticsearch.password"
	PathLoggingESToken        = "system_config.logging.elasticsearch.token"
	PathLoggingLokiToken      = "system_config.logging.loki.token"
	PathLoggingWebhookToken   = "system_config.logging.webhook.token"
)
