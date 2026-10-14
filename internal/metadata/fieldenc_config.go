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
// Vault secret_id and token are sealed even when field encryption is off.
func EncryptSystemConfigPaths(svc FieldEncrypter, cfg SystemConfig) (SystemConfig, error) {
	return encryptSystemConfigPaths(svc, deriveKMSSealKey(defaultKMSSealMaterial()), cfg)
}

// EncryptSystemConfigWithKey is EncryptSystemConfigPaths with an explicit seal key.
func EncryptSystemConfigWithKey(svc FieldEncrypter, key []byte, cfg SystemConfig) (SystemConfig, error) {
	return encryptSystemConfigPaths(svc, key, cfg)
}

// DecryptSystemConfigWithKey is DecryptSystemConfigPaths with an explicit seal key.
func DecryptSystemConfigWithKey(svc FieldEncrypter, key []byte, cfg SystemConfig) (SystemConfig, error) {
	return decryptSystemConfigPaths(svc, key, cfg)
}

func encryptSystemConfigPaths(svc FieldEncrypter, key []byte, cfg SystemConfig) (SystemConfig, error) {
	if svc != nil && svc.Enabled() {
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
		if cfg.KMS.SecretID, err = rewrap(svc, fieldenc.PathKMSVaultSecretID, cfg.KMS.SecretID); err != nil {
			return cfg, err
		}
		if cfg.KMS.Token, err = rewrap(svc, fieldenc.PathKMSVaultToken, cfg.KMS.Token); err != nil {
			return cfg, err
		}
		if cfg.Logging, err = encryptLoggingConfig(svc, cfg.Logging); err != nil {
			return cfg, err
		}
	}
	sealed, err := sealKMSConfig(key, cfg.KMS)
	if err != nil {
		return cfg, err
	}
	cfg.KMS = sealed
	return cfg, nil
}

// DecryptSystemConfigPaths decrypts secret leaves after load.
func DecryptSystemConfigPaths(svc FieldEncrypter, cfg SystemConfig) (SystemConfig, error) {
	return decryptSystemConfigPaths(svc, deriveKMSSealKey(defaultKMSSealMaterial()), cfg)
}

func decryptSystemConfigPaths(svc FieldEncrypter, key []byte, cfg SystemConfig) (SystemConfig, error) {
	if svc != nil {
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
		if cfg.KMS.SecretID, err = svc.Decrypt(fieldenc.PathKMSVaultSecretID, cfg.KMS.SecretID); err != nil {
			return cfg, err
		}
		if cfg.KMS.Token, err = svc.Decrypt(fieldenc.PathKMSVaultToken, cfg.KMS.Token); err != nil {
			return cfg, err
		}
		if cfg.Logging, err = decryptLoggingConfig(svc, cfg.Logging); err != nil {
			return cfg, err
		}
	}
	opened, err := openKMSConfig(key, cfg.KMS)
	if err != nil {
		return cfg, err
	}
	cfg.KMS = opened
	return cfg, nil
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
