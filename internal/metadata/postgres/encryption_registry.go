package postgres

import (
	"context"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListEncryptionKeys(ctx context.Context) ([]metadata.EncryptionKeyRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT kek_id, algorithm, public_key, is_active, created_at, rotated_at, retired_at
		FROM encryption_key_registry
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.EncryptionKeyRecord
	for rows.Next() {
		var rec metadata.EncryptionKeyRecord
		var rotatedAt, retiredAt *time.Time
		if err := rows.Scan(&rec.KEKID, &rec.Algorithm, &rec.PublicKey, &rec.IsActive, &rec.CreatedAt, &rotatedAt, &retiredAt); err != nil {
			return nil, err
		}
		rec.RotatedAt = rotatedAt
		rec.RetiredAt = retiredAt
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) GetEncryptionKey(ctx context.Context, kekID string) (metadata.EncryptionKeyRecord, error) {
	var rec metadata.EncryptionKeyRecord
	var rotatedAt, retiredAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT kek_id, algorithm, public_key, is_active, created_at, rotated_at, retired_at
		FROM encryption_key_registry WHERE kek_id=$1`, kekID).Scan(
		&rec.KEKID, &rec.Algorithm, &rec.PublicKey, &rec.IsActive, &rec.CreatedAt, &rotatedAt, &retiredAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return rec, metadata.ErrNotFound
		}
		return rec, err
	}
	rec.RotatedAt = rotatedAt
	rec.RetiredAt = retiredAt
	return rec, nil
}

func (s *Store) GetActiveEncryptionKey(ctx context.Context) (metadata.EncryptionKeyRecord, error) {
	var rec metadata.EncryptionKeyRecord
	var rotatedAt, retiredAt *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT kek_id, algorithm, public_key, is_active, created_at, rotated_at, retired_at
		FROM encryption_key_registry
		WHERE is_active=TRUE AND retired_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1`).Scan(
		&rec.KEKID, &rec.Algorithm, &rec.PublicKey, &rec.IsActive, &rec.CreatedAt, &rotatedAt, &retiredAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return rec, metadata.ErrNotFound
		}
		return rec, err
	}
	rec.RotatedAt = rotatedAt
	rec.RetiredAt = retiredAt
	return rec, nil
}

func (s *Store) InsertEncryptionKey(ctx context.Context, rec metadata.EncryptionKeyRecord) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	if rec.Algorithm == "" {
		rec.Algorithm = metadata.EncryptionAlgorithmX25519
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO encryption_key_registry (kek_id, algorithm, public_key, is_active, created_at, rotated_at, retired_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (kek_id) DO NOTHING`,
		rec.KEKID, rec.Algorithm, rec.PublicKey, rec.IsActive, rec.CreatedAt,
		timestamptzPtr(rec.RotatedAt), timestamptzPtr(rec.RetiredAt))
	return err
}

func (s *Store) SetEncryptionKeyActive(ctx context.Context, kekID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE encryption_key_registry
		SET is_active=FALSE, rotated_at=COALESCE(rotated_at, $2)
		WHERE is_active=TRUE AND kek_id<>$1 AND retired_at IS NULL`, kekID, now)
	if err != nil {
		return err
	}
	_ = tag
	res, err := tx.Exec(ctx, `
		UPDATE encryption_key_registry SET is_active=TRUE WHERE kek_id=$1 AND retired_at IS NULL`, kekID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return tx.Commit(ctx)
}

var _ metadata.EncryptionKeyRegistry = (*Store)(nil)
