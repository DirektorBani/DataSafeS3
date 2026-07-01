package metadata

import (
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

// FieldEncrypter is the subset of fieldenc.Service used by metadata stores.
type FieldEncrypter interface {
	Enabled() bool
	Decrypt(fieldPath, stored string) (string, error)
	RewrapIfNeeded(fieldPath, stored string) (string, bool, error)
}

// EncryptSystemConfigPaths encrypts or rewraps secret leaves before persistence.
func EncryptSystemConfigPaths(svc FieldEncrypter, cfg SystemConfig) (SystemConfig, error) {
	if svc == nil || !svc.Enabled() {
		return cfg, nil
	}
	var err error
	if cfg.LDAP.BindPassword, err = rewrap(svc, fieldenc.PathLDAPBindPassword, cfg.LDAP.BindPassword); err != nil {
		return cfg, err
	}
	if cfg.OIDC.ClientSecret, err = rewrap(svc, fieldenc.PathOIDCClientSecret, cfg.OIDC.ClientSecret); err != nil {
		return cfg, err
	}
	if cfg.ExternalS3.SecretAccessKey, err = rewrap(svc, fieldenc.PathExternalS3Secret, cfg.ExternalS3.SecretAccessKey); err != nil {
		return cfg, err
	}
	cfg.Logging, err = encryptLoggingConfig(svc, cfg.Logging)
	return cfg, err
}

// DecryptSystemConfigPaths decrypts secret leaves after load.
func DecryptSystemConfigPaths(svc FieldEncrypter, cfg SystemConfig) (SystemConfig, error) {
	if svc == nil {
		return cfg, nil
	}
	var err error
	if cfg.LDAP.BindPassword, err = svc.Decrypt(fieldenc.PathLDAPBindPassword, cfg.LDAP.BindPassword); err != nil {
		return cfg, err
	}
	if cfg.OIDC.ClientSecret, err = svc.Decrypt(fieldenc.PathOIDCClientSecret, cfg.OIDC.ClientSecret); err != nil {
		return cfg, err
	}
	if cfg.ExternalS3.SecretAccessKey, err = svc.Decrypt(fieldenc.PathExternalS3Secret, cfg.ExternalS3.SecretAccessKey); err != nil {
		return cfg, err
	}
	cfg.Logging, err = decryptLoggingConfig(svc, cfg.Logging)
	return cfg, err
}

func rewrap(svc FieldEncrypter, path, val string) (string, error) {
	out, _, err := svc.RewrapIfNeeded(path, val)
	return out, err
}

func encryptLoggingConfig(svc FieldEncrypter, cfg LoggingConfig) (LoggingConfig, error) {
	var err error
	if cfg.Elasticsearch.Password, err = rewrap(svc, fieldenc.PathLoggingESPassword, cfg.Elasticsearch.Password); err != nil {
		return cfg, err
	}
	if cfg.Elasticsearch.Token, err = rewrap(svc, fieldenc.PathLoggingESToken, cfg.Elasticsearch.Token); err != nil {
		return cfg, err
	}
	if cfg.Loki.Token, err = rewrap(svc, fieldenc.PathLoggingLokiToken, cfg.Loki.Token); err != nil {
		return cfg, err
	}
	if cfg.Webhook.Token, err = rewrap(svc, fieldenc.PathLoggingWebhookToken, cfg.Webhook.Token); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func decryptLoggingConfig(svc FieldEncrypter, cfg LoggingConfig) (LoggingConfig, error) {
	var err error
	if cfg.Elasticsearch.Password, err = svc.Decrypt(fieldenc.PathLoggingESPassword, cfg.Elasticsearch.Password); err != nil {
		return cfg, err
	}
	if cfg.Elasticsearch.Token, err = svc.Decrypt(fieldenc.PathLoggingESToken, cfg.Elasticsearch.Token); err != nil {
		return cfg, err
	}
	if cfg.Loki.Token, err = svc.Decrypt(fieldenc.PathLoggingLokiToken, cfg.Loki.Token); err != nil {
		return cfg, err
	}
	if cfg.Webhook.Token, err = svc.Decrypt(fieldenc.PathLoggingWebhookToken, cfg.Webhook.Token); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// FieldEncryptionReporter exposes registry metadata for security-status.
type FieldEncryptionReporter interface {
	EncryptionRegistryCount() int
	FieldEnc() *fieldenc.Service
}
