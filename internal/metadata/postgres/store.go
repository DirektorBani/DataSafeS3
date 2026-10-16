package postgres

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	"sync"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationAdvisoryLock is a session lock held for the whole migration pass.
// Distinct from outboxAdvisoryLock. Several nodes starting on an empty database
// otherwise race in CREATE EXTENSION (pg_extension_name_index).
const migrationAdvisoryLock int64 = 83420021

const postgresPingAttempts = 30

func pingPostgres(pool *pgxpool.Pool) error {
	var last error
	for attempt := 1; attempt <= postgresPingAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		last = pool.Ping(ctx)
		cancel()
		if last == nil {
			return nil
		}
		if attempt < postgresPingAttempts {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}
	return last
}

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store implements metadata.MetadataStore using PostgreSQL.
type Store struct {
	pool             *pgxpool.Pool
	readPool         *pgxpool.Pool
	fieldenc         *fieldenc.Service
	kmsSealKey       []byte
	outboxMaxPending int
	outboxMaxSet     bool
	// sticky is the schema-lock connection. Seed runs its own statements on
	// that session so startup does not dial a second one.
	stickyMu sync.Mutex
	sticky   pgExec
}

// Open connects to PostgreSQL, runs migrations, and returns a Store.
// readReplicaDSN is optional; when set, list-style reads route to the replica.
func Open(dsn string, readReplicaDSN string) (*Store, error) {
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres connect: %w", err)
	}
	if err := pingPostgres(pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	s := &Store{pool: pool}
	if readReplicaDSN != "" && readReplicaDSN != dsn {
		readPool, err := pgxpool.New(context.Background(), readReplicaDSN)
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("postgres read replica connect: %w", err)
		}
		if err := pingPostgres(readPool); err != nil {
			readPool.Close()
			pool.Close()
			return nil, fmt.Errorf("postgres read replica ping: %w", err)
		}
		s.readPool = readPool
	}
	if err := s.migrate(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// SetFieldEncryption attaches the field encryption service (injected at startup).
func (s *Store) SetFieldEncryption(fe *fieldenc.Service) {
	s.fieldenc = fe
}

// SetKMSSealMaterial seals Vault secrets when field encryption is off.
func (s *Store) SetKMSSealMaterial(material string) {
	if s == nil {
		return
	}
	s.kmsSealKey = metadata.DeriveKMSSealKey(material)
}

func (s *Store) kmsKey() []byte {
	if s != nil && len(s.kmsSealKey) == 32 {
		return s.kmsSealKey
	}
	return metadata.DeriveKMSSealKey("")
}

func (s *Store) readQueryPool() *pgxpool.Pool {
	if s.readPool != nil {
		return s.readPool
	}
	return s.pool
}

func (s *Store) Close() error {
	if s.readPool != nil {
		s.readPool.Close()
		s.readPool = nil
	}
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
	return nil
}

type sqlExec interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// WithSchemaLock holds the same session advisory lock as migrations.
// Bootstrap and other one-time writes use it so several nodes can start
// together on an empty database.
func (s *Store) WithSchemaLock(ctx context.Context, fn func() error) error {
	if fn == nil {
		return nil
	}
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("schema lock: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLock); err != nil {
		return fmt.Errorf("schema lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLock)
	}()
	s.stickyMu.Lock()
	s.sticky = conn
	s.stickyMu.Unlock()
	defer func() {
		s.stickyMu.Lock()
		s.sticky = nil
		s.stickyMu.Unlock()
	}()
	return fn()
}

// queryExec returns the schema-lock session while WithSchemaLock is running.
func (s *Store) queryExec() pgExec {
	s.stickyMu.Lock()
	defer s.stickyMu.Unlock()
	if s.sticky != nil {
		return s.sticky
	}
	return s.pool
}

func (s *Store) migrate() error {
	ctx := context.Background()
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrationAdvisoryLock); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationAdvisoryLock)
	}()
	return s.migrateLocked(ctx, conn)
}

func (s *Store) migrateLocked(ctx context.Context, conn *pgxpool.Conn) error {
	var version int64
	err := conn.QueryRow(ctx, `SELECT COALESCE(MAX(version), 0) FROM schema_migrations WHERE dirty = FALSE`).Scan(&version)
	if err != nil {
		version = 0
	}
	entries, err := fs.Glob(migrationFS, "migrations/*.up.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, path := range entries {
		ver := migrationVersion(path)
		if ver <= version {
			continue
		}
		body, err := migrationFS.ReadFile(path)
		if err != nil {
			return err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("migration %s: %w", path, err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version, dirty) VALUES ($1, FALSE) ON CONFLICT (version) DO UPDATE SET dirty = FALSE`, ver); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return s.backfillBucketOwnersQ(ctx, conn)
}

func (s *Store) backfillBucketOwners(ctx context.Context) error {
	return s.backfillBucketOwnersQ(ctx, s.pool)
}

func (s *Store) backfillBucketOwnersQ(ctx context.Context, q sqlExec) error {
	// Backfill owner_id from username; skip rows that would violate idx_buckets_scope_owner_name.
	_, err := q.Exec(ctx, `
		UPDATE buckets b
		SET owner_id = u.id
		FROM users u
		WHERE (b.owner_id IS NULL OR b.owner_id = '')
		  AND b.owner <> ''
		  AND b.owner = u.username
		  AND NOT EXISTS (
		    SELECT 1 FROM buckets b2
		    WHERE b2.storage_key <> b.storage_key
		      AND COALESCE(b2.owner_id, '') = u.id
		      AND b2.name = b.name
		      AND (b2.tenant_id IS NULL OR b2.tenant_id = '' OR b2.tenant_id = 'default')
		  )`)
	if err != nil {
		return err
	}
	// Drop empty legacy orphans superseded by a scoped bucket for the same owner/name.
	_, err = q.Exec(ctx, `
		DELETE FROM buckets b
		WHERE (b.owner_id IS NULL OR b.owner_id = '')
		  AND b.owner <> ''
		  AND EXISTS (
		    SELECT 1 FROM buckets b2
		    JOIN users u ON u.username = b.owner
		    WHERE b2.storage_key <> b.storage_key
		      AND COALESCE(b2.owner_id, '') = u.id
		      AND b2.name = b.name
		      AND (b2.tenant_id IS NULL OR b2.tenant_id = '' OR b2.tenant_id = 'default')
		  )
		  AND NOT EXISTS (
		    SELECT 1 FROM objects o
		    WHERE o.bucket = b.storage_key
		      AND o.is_latest = TRUE
		      AND o.is_delete_marker = FALSE
		  )`)
	return err
}

func migrationVersion(path string) int64 {
	name := strings.TrimPrefix(path, "migrations/")
	var num strings.Builder
	for _, ch := range name {
		if ch >= '0' && ch <= '9' {
			num.WriteRune(ch)
		} else {
			break
		}
	}
	v, _ := strconv.ParseInt(num.String(), 10, 64)
	return v
}

var _ metadata.MetadataStore = (*Store)(nil)
var _ metadata.OutboxMutator = (*Store)(nil)
var _ events.OutboxStore = (*Store)(nil)
