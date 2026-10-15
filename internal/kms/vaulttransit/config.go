// Package vaulttransit talks to HashiCorp Vault Transit over HTTP.
// The client is created only when the provider is vault-transit and an address is set.
package vaulttransit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

const (
	defaultMount    = "transit"
	defaultCacheTTL = 5 * time.Minute
	defaultPoll     = 2 * time.Second
	defaultTimeout  = 5 * time.Second
	defaultCacheMax = 256
)

// Config is one node's Vault Transit connection.
// A per-node AppRole or token from the environment overrides the shared secret.
type Config struct {
	Provider          string
	Addr              string
	Namespace         string
	Mount             string
	CACert            string
	RoleID            string
	SecretID          string
	SecretIDFile      string
	Token             string
	TokenFile         string
	AdminRoleID       string
	AdminSecretID     string
	AdminSecretIDFile string
	Timeout           time.Duration
	CacheTTL          time.Duration
	PollInterval      time.Duration
	CacheMax          int
	// ProbeKey is an existing Transit key for the connection test.
	// STORAGE_KMS_PROBE_KEY wins. STORAGE_KMS_DEFAULT_KEY is the fallback.
	// Empty means the test creates datasafe-connection-test and does not delete it.
	ProbeKey string
}

// EnvLocks marks fields the environment set. The console must not edit them.
type EnvLocks struct {
	Provider  bool `json:"provider,omitempty"`
	Addr      bool `json:"addr,omitempty"`
	Namespace bool `json:"namespace,omitempty"`
	Mount     bool `json:"mount,omitempty"`
	CACert    bool `json:"ca_cert,omitempty"`
	RoleID    bool `json:"role_id,omitempty"`
	Secret    bool `json:"secret,omitempty"`
	Token     bool `json:"token,omitempty"`
}

// Enabled is true only for provider vault-transit with an address.
// Anything else must not open a connection.
func (c Config) Enabled() bool {
	return strings.EqualFold(strings.TrimSpace(c.Provider), kms.ProviderVault) && strings.TrimSpace(c.Addr) != ""
}

// Fingerprint identifies the connection material. It stays in memory and is not logged.
func (c Config) Fingerprint() string {
	h := sha256.New()
	parts := []string{
		c.Provider, c.Addr, c.Namespace, c.Mount, c.CACert, c.RoleID, c.SecretID, c.SecretIDFile,
		c.Token, c.TokenFile, c.AdminRoleID, c.AdminSecretID, c.AdminSecretIDFile,
		c.ProbeKey, c.Timeout.String(), c.CacheTTL.String(), c.PollInterval.String(),
	}
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// LoadEnv reads STORAGE_KMS_* . Empty values are unset.
func LoadEnv() (Config, EnvLocks) {
	var locks EnvLocks
	cfg := Config{
		Provider:          getenv("STORAGE_KMS_PROVIDER", &locks.Provider),
		Addr:              normalizeAddr(getenv("STORAGE_KMS_VAULT_ADDR", &locks.Addr)),
		Namespace:         getenv("STORAGE_KMS_VAULT_NAMESPACE", &locks.Namespace),
		Mount:             getenv("STORAGE_KMS_VAULT_TRANSIT_MOUNT", &locks.Mount),
		CACert:            getenv("STORAGE_KMS_VAULT_CACERT", &locks.CACert),
		RoleID:            getenv("STORAGE_KMS_VAULT_ROLE_ID", &locks.RoleID),
		SecretID:          os.Getenv("STORAGE_KMS_VAULT_SECRET_ID"),
		SecretIDFile:      os.Getenv("STORAGE_KMS_VAULT_SECRET_ID_FILE"),
		TokenFile:         os.Getenv("STORAGE_KMS_VAULT_TOKEN_FILE"),
		AdminRoleID:       strings.TrimSpace(os.Getenv("STORAGE_KMS_VAULT_ADMIN_ROLE_ID")),
		AdminSecretID:     os.Getenv("STORAGE_KMS_VAULT_ADMIN_SECRET_ID"),
		AdminSecretIDFile: strings.TrimSpace(os.Getenv("STORAGE_KMS_VAULT_ADMIN_SECRET_ID_FILE")),
		Timeout:           durEnv("STORAGE_KMS_TIMEOUT", defaultTimeout),
		CacheTTL:          durEnv("STORAGE_KMS_DEK_CACHE_TTL", defaultCacheTTL),
		PollInterval:      durEnv("STORAGE_KMS_POLL_INTERVAL", defaultPoll),
		CacheMax:          defaultCacheMax,
		ProbeKey:          probeKeyFromEnv(),
	}
	cfg.SecretID = strings.TrimSpace(cfg.SecretID)
	cfg.SecretIDFile = strings.TrimSpace(cfg.SecretIDFile)
	cfg.TokenFile = strings.TrimSpace(cfg.TokenFile)
	cfg.AdminSecretID = strings.TrimSpace(cfg.AdminSecretID)
	if cfg.SecretID != "" || cfg.SecretIDFile != "" {
		locks.Secret = true
	}
	if cfg.TokenFile != "" {
		locks.Token = true
	}
	cfg.applyDefaults()
	return cfg, locks
}

// Merge lets environment fields win over the console-saved config.
func Merge(env Config, locks EnvLocks, stored metadata.VaultKMSConfig) Config {
	out := env
	if !locks.Provider && strings.TrimSpace(stored.Provider) != "" {
		out.Provider = strings.TrimSpace(stored.Provider)
	}
	if !locks.Addr && strings.TrimSpace(stored.Addr) != "" {
		out.Addr = normalizeAddr(stored.Addr)
	}
	if !locks.Namespace && strings.TrimSpace(stored.Namespace) != "" {
		out.Namespace = strings.TrimSpace(stored.Namespace)
	}
	if !locks.Mount && strings.TrimSpace(stored.TransitMount) != "" {
		out.Mount = strings.TrimSpace(stored.TransitMount)
	}
	if !locks.CACert && strings.TrimSpace(stored.CACert) != "" {
		out.CACert = stored.CACert
	}
	envCreds := locks.Secret || locks.Token
	if !envCreds {
		if strings.EqualFold(stored.Auth, "token") {
			out.Token = stored.Token
		} else {
			if !locks.RoleID && strings.TrimSpace(stored.RoleID) != "" {
				out.RoleID = strings.TrimSpace(stored.RoleID)
			}
			out.SecretID = stored.SecretID
		}
	} else if !locks.RoleID && strings.TrimSpace(stored.RoleID) != "" && out.RoleID == "" {
		out.RoleID = strings.TrimSpace(stored.RoleID)
	}
	out.applyDefaults()
	return out
}

func (c *Config) applyDefaults() {
	if strings.TrimSpace(c.Mount) == "" {
		c.Mount = defaultMount
	}
	if c.Timeout <= 0 {
		c.Timeout = defaultTimeout
	}
	if c.CacheTTL <= 0 {
		c.CacheTTL = defaultCacheTTL
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPoll
	}
	if c.CacheMax <= 0 {
		c.CacheMax = defaultCacheMax
	}
	c.Addr = normalizeAddr(c.Addr)
}

func probeKeyFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("STORAGE_KMS_PROBE_KEY")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("STORAGE_KMS_DEFAULT_KEY"))
}

func getenv(key string, locked *bool) string {
	v, ok := os.LookupEnv(key)
	v = strings.TrimSpace(v)
	if ok && v != "" {
		*locked = true
		return v
	}
	return ""
}

func durEnv(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func normalizeAddr(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}
