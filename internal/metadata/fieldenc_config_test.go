package metadata

import (
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
)

func testFieldEncSvc(t *testing.T) *fieldenc.Service {
	t.Helper()
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	svc, err := fieldenc.NewForTest("kek-config-test", seed, true)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestEncryptSystemConfigPaths_secretLeaves(t *testing.T) {
	svc := testFieldEncSvc(t)
	cfg := SystemConfig{
		LDAP:       LDAPConfig{BindPassword: "ldap-bind"},
		OIDC:       OIDCConfig{ClientSecret: "oidc-secret"},
		ExternalS3: ExternalS3Config{SecretAccessKey: "s3-secret"},
		Logging: LoggingConfig{
			Elasticsearch: LogSinkEndpoint{Password: "es-pass", Token: "es-token"},
			Loki:          LogSinkEndpoint{Token: "loki-token"},
			Webhook:       LogSinkEndpoint{Token: "wh-token"},
		},
	}

	enc, err := EncryptSystemConfigPaths(svc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for name, val := range map[string]string{
		"ldap":       enc.LDAP.BindPassword,
		"oidc":       enc.OIDC.ClientSecret,
		"external_s3": enc.ExternalS3.SecretAccessKey,
		"es_pass":    enc.Logging.Elasticsearch.Password,
		"es_token":   enc.Logging.Elasticsearch.Token,
		"loki":       enc.Logging.Loki.Token,
		"webhook":    enc.Logging.Webhook.Token,
	} {
		if !strings.HasPrefix(val, "enc:v1:") {
			t.Fatalf("%s: expected enc:v1, got %q", name, val)
		}
	}

	dec, err := DecryptSystemConfigPaths(svc, enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec.LDAP.BindPassword != cfg.LDAP.BindPassword ||
		dec.OIDC.ClientSecret != cfg.OIDC.ClientSecret ||
		dec.ExternalS3.SecretAccessKey != cfg.ExternalS3.SecretAccessKey ||
		dec.Logging.Elasticsearch.Password != cfg.Logging.Elasticsearch.Password ||
		dec.Logging.Elasticsearch.Token != cfg.Logging.Elasticsearch.Token ||
		dec.Logging.Loki.Token != cfg.Logging.Loki.Token ||
		dec.Logging.Webhook.Token != cfg.Logging.Webhook.Token {
		t.Fatalf("roundtrip mismatch: %+v", dec)
	}
}

func TestEncryptSystemConfigPaths_disabledPassthrough(t *testing.T) {
	svc, err := fieldenc.NewForTest("kek-off", make([]byte, 32), false)
	if err != nil {
		t.Fatal(err)
	}
	cfg := SystemConfig{LDAP: LDAPConfig{BindPassword: "plain"}}
	out, err := EncryptSystemConfigPaths(svc, cfg)
	if err != nil || out.LDAP.BindPassword != "plain" {
		t.Fatalf("disabled: %+v err=%v", out, err)
	}
}
