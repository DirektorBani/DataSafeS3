package postgres

import (
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func init() {
	metadata.RegisterPostgresOpener(openFromConfig)
}

func openFromConfig(cfg metadata.Config) (metadata.MetadataStore, error) {
	dsn := cfg.PostgresDSN
	if dsn == "" {
		dsn = metadata.PostgresDSN(cfg)
	}
	s, err := Open(dsn, cfg.PostgresReadReplicaDSN)
	if err != nil {
		return nil, err
	}
	if cfg.FieldEncryption != nil {
		s.SetFieldEncryption(cfg.FieldEncryption)
	}
	return s, nil
}

// DSNFromConfig returns the resolved PostgreSQL connection string.
func DSNFromConfig(cfg metadata.Config) string {
	return metadata.PostgresDSN(cfg)
}

// OpenFromConfig connects using metadata.Config fields.
func OpenFromConfig(cfg metadata.Config) (*Store, error) {
	return Open(metadata.PostgresDSN(cfg), cfg.PostgresReadReplicaDSN)
}

func mustDSN(cfg metadata.Config) string {
	dsn := cfg.PostgresDSN
	if dsn == "" {
		dsn = metadata.PostgresDSN(cfg)
	}
	if dsn == "" {
		panic("empty postgres dsn")
	}
	return dsn
}

// ResolveDSN is exported for CLI tools.
func ResolveDSN(cfg metadata.Config) string {
	return metadata.PostgresDSN(cfg)
}
