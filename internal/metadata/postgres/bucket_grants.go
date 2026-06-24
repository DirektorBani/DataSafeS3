package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Store) PutBucketAccessGrant(grant metadata.BucketAccessGrant) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO bucket_access_grants (bucket_key, user_id, can_read, can_write)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (bucket_key, user_id) DO UPDATE SET can_read=$3, can_write=$4`,
		grant.BucketKey, grant.UserID, grant.CanRead, grant.CanWrite)
	return err
}

func (s *Store) ListBucketAccessGrants(bucketKey string) ([]metadata.BucketAccessGrant, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT bucket_key, user_id, can_read, can_write
		FROM bucket_access_grants WHERE bucket_key=$1 ORDER BY user_id`, bucketKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.BucketAccessGrant
	for rows.Next() {
		var g metadata.BucketAccessGrant
		if err := rows.Scan(&g.BucketKey, &g.UserID, &g.CanRead, &g.CanWrite); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBucketAccessGrant(bucketKey, userID string) error {
	tag, err := s.pool.Exec(context.Background(), `
		DELETE FROM bucket_access_grants WHERE bucket_key=$1 AND user_id=$2`, bucketKey, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ReplaceBucketAccessGrants(bucketKey string, grants []metadata.BucketAccessGrant) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM bucket_access_grants WHERE bucket_key=$1`, bucketKey); err != nil {
		return err
	}
	for _, g := range grants {
		if _, err := tx.Exec(ctx, `
			INSERT INTO bucket_access_grants (bucket_key, user_id, can_read, can_write)
			VALUES ($1,$2,$3,$4)`, bucketKey, g.UserID, g.CanRead, g.CanWrite); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) CountBucketAccessGrants(bucketKey string) (int, error) {
	var n int
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM bucket_access_grants WHERE bucket_key=$1`, bucketKey).Scan(&n)
	return n, err
}
