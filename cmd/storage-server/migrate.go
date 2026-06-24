package main

import (
	"fmt"
	"path/filepath"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/metadata/postgres"
)

func runMigrateBolt(args []string) error {
	dataDir := envOr("STORAGE_DATA_DIR", "./data")
	boltPath := filepath.Join(dataDir, "metadata.db")
	cfg := metadata.ConfigFromEnv(dataDir)
	dsn := postgres.ResolveDSN(cfg)
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--bolt-path":
			if i+1 < len(args) {
				boltPath = args[i+1]
				i++
			}
		case "--postgres-dsn":
			if i+1 < len(args) {
				dsn = args[i+1]
				i++
			}
		}
	}
	if dsn == "" {
		return fmt.Errorf("STORAGE_POSTGRES_DSN or postgres connection vars required")
	}
	report, err := postgres.MigrateBoltToPostgres(boltPath, dsn)
	if err != nil {
		return err
	}
	fmt.Printf("Migration complete: buckets=%d objects=%d users=%d keys=%d webhooks=%d audit=%d skipped=%d errors=%d idempotent=%v\n",
		report.Buckets, report.Objects, report.Users, report.AccessKeys, report.Webhooks,
		report.AuditLogs, report.Skipped, report.Errors, report.IdempotentOK)
	if report.Errors > 0 {
		return fmt.Errorf("migration finished with %d errors", report.Errors)
	}
	return nil
}
