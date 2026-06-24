package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Backend selects the metadata persistence engine.
type Backend string

const (
	BackendBolt     Backend = "bolt"
	BackendPostgres Backend = "postgres"
)

// Config holds metadata store connection settings.
type Config struct {
	Backend Backend
	DataDir string

	PostgresDSN            string
	PostgresReadReplicaDSN string
	PostgresHost           string
	PostgresPort           string
	PostgresUser           string
	PostgresPassword       string
	PostgresDB             string
}

// ConfigFromEnv builds Config from environment variables.
func ConfigFromEnv(dataDir string) Config {
	cfg := Config{
		Backend: BackendBolt,
		DataDir: dataDir,
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_METADATA_BACKEND"))); v != "" {
		cfg.Backend = Backend(v)
	}
	cfg.PostgresDSN = os.Getenv("STORAGE_POSTGRES_DSN")
	cfg.PostgresReadReplicaDSN = os.Getenv("STORAGE_POSTGRES_READ_REPLICA_DSN")
	cfg.PostgresHost = envDefault("STORAGE_POSTGRES_HOST", "localhost")
	cfg.PostgresPort = envDefault("STORAGE_POSTGRES_PORT", "5432")
	cfg.PostgresUser = envDefault("STORAGE_POSTGRES_USER", "datasafe")
	cfg.PostgresPassword = os.Getenv("STORAGE_POSTGRES_PASSWORD")
	cfg.PostgresDB = envDefault("STORAGE_POSTGRES_DB", "datasafe")
	return cfg
}

func envDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// PostgresOpener is registered by internal/metadata/postgres at init time.
var PostgresOpener func(cfg Config) (MetadataStore, error)

// RegisterPostgresOpener wires the PostgreSQL backend without an import cycle.
func RegisterPostgresOpener(fn func(cfg Config) (MetadataStore, error)) {
	PostgresOpener = fn
}

// Open selects and opens the configured metadata backend.
func Open(cfg Config) (MetadataStore, error) {
	switch cfg.Backend {
	case "", BackendBolt:
		if cfg.DataDir == "" {
			cfg.DataDir = "./data"
		}
		return OpenBolt(filepath.Join(cfg.DataDir, "metadata.db"))
	case BackendPostgres:
		if PostgresOpener == nil {
			return nil, fmt.Errorf("postgres backend not available (import postgres package)")
		}
		return PostgresOpener(cfg)
	default:
		return nil, fmt.Errorf("unsupported metadata backend %q (use bolt or postgres)", cfg.Backend)
	}
}

// OpenBolt opens the embedded BoltDB metadata store.
func OpenBolt(path string) (*Store, error) {
	return openBolt(path)
}

func openBolt(path string) (*Store, error) {
	db, err := boltOpen(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// PostgresDSN builds a connection string from Config fields.
func PostgresDSN(cfg Config) string {
	if cfg.PostgresDSN != "" {
		return cfg.PostgresDSN
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		cfg.PostgresUser,
		cfg.PostgresPassword,
		cfg.PostgresHost,
		cfg.PostgresPort,
		cfg.PostgresDB,
	)
}

// NormalizeBackend returns a validated backend name.
func NormalizeBackend(v string) Backend {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "postgres", "postgresql", "pg":
		return BackendPostgres
	default:
		return BackendBolt
	}
}
