package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Store) PutBucketPrefixAccessGrant(grant metadata.BucketPrefixAccessGrant) error {
	grant.Prefix = metadata.NormalizeSharePrefix(grant.Prefix)
	if grant.Prefix == "" {
		return metadata.ErrInvalidArgument
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO bucket_prefix_access_grants (bucket_key, user_id, prefix, can_read, can_write)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (bucket_key, user_id, prefix) DO UPDATE SET can_read=$4, can_write=$5`,
		grant.BucketKey, grant.UserID, grant.Prefix, grant.CanRead, grant.CanWrite)
	return err
}

func (s *Store) ListBucketPrefixAccessGrants(bucketKey string) ([]metadata.BucketPrefixAccessGrant, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT bucket_key, user_id, prefix, can_read, can_write
		FROM bucket_prefix_access_grants WHERE bucket_key=$1 ORDER BY user_id, prefix`, bucketKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.BucketPrefixAccessGrant
	for rows.Next() {
		var g metadata.BucketPrefixAccessGrant
		if err := rows.Scan(&g.BucketKey, &g.UserID, &g.Prefix, &g.CanRead, &g.CanWrite); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) ListUserPrefixAccessGrants(userID string) ([]metadata.BucketPrefixAccessGrant, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT bucket_key, user_id, prefix, can_read, can_write
		FROM bucket_prefix_access_grants WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.BucketPrefixAccessGrant
	for rows.Next() {
		var g metadata.BucketPrefixAccessGrant
		if err := rows.Scan(&g.BucketKey, &g.UserID, &g.Prefix, &g.CanRead, &g.CanWrite); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) DeleteBucketPrefixAccessGrant(bucketKey, userID, prefix string) error {
	tag, err := s.pool.Exec(context.Background(), `
		DELETE FROM bucket_prefix_access_grants
		WHERE bucket_key=$1 AND user_id=$2 AND prefix=$3`,
		bucketKey, userID, metadata.NormalizeSharePrefix(prefix))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteBucketPrefixAccessGrantsForUser(bucketKey, userID string) error {
	_, err := s.pool.Exec(context.Background(), `
		DELETE FROM bucket_prefix_access_grants WHERE bucket_key=$1 AND user_id=$2`,
		bucketKey, userID)
	return err
}

func (s *Store) ReplaceBucketPrefixAccessGrants(bucketKey string, grants []metadata.BucketPrefixAccessGrant) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM bucket_prefix_access_grants WHERE bucket_key=$1`, bucketKey); err != nil {
		return err
	}
	for _, g := range grants {
		g.Prefix = metadata.NormalizeSharePrefix(g.Prefix)
		if g.Prefix == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO bucket_prefix_access_grants (bucket_key, user_id, prefix, can_read, can_write)
			VALUES ($1,$2,$3,$4,$5)`, bucketKey, g.UserID, g.Prefix, g.CanRead, g.CanWrite); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) CountBucketPrefixAccessGrants(bucketKey string) (int, error) {
	var n int
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM bucket_prefix_access_grants WHERE bucket_key=$1`, bucketKey).Scan(&n)
	return n, err
}

func (s *Store) PutUserNotification(rec metadata.UserNotificationRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO user_notifications (id, user_id, kind, title, body, link, read_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET title=$4, body=$5, link=$6`,
		rec.ID, rec.UserID, rec.Kind, rec.Title, rec.Body, rec.Link, rec.ReadAt, rec.CreatedAt)
	return err
}

func (s *Store) ListUserNotifications(userID string, limit int) ([]metadata.UserNotificationRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, user_id, kind, title, body, link, read_at, created_at
		FROM user_notifications WHERE user_id=$1 ORDER BY created_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanNotifications(rows)
}

func (s *Store) MarkUserNotificationRead(userID, id string) error {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE user_notifications SET read_at=NOW() WHERE user_id=$1 AND id=$2`, userID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllUserNotificationsRead(userID string) error {
	_, err := s.pool.Exec(context.Background(), `
		UPDATE user_notifications SET read_at=NOW()
		WHERE user_id=$1 AND read_at IS NULL`, userID)
	return err
}

func (s *Store) CountUnreadNotifications(userID string) (int, error) {
	var n int
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM user_notifications WHERE user_id=$1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// recentItemIDSep separates fields in recent_items.id (Postgres TEXT cannot contain NUL).
const recentItemIDSep = "\x1e"

func (s *Store) TouchRecentItem(userID, bucket, prefix string) error {
	id := userID + recentItemIDSep + bucket + recentItemIDSep + prefix
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO recent_items (id, user_id, bucket, prefix, accessed_at)
		VALUES ($1,$2,$3,$4,NOW())
		ON CONFLICT (id) DO UPDATE SET accessed_at=NOW()`,
		id, userID, bucket, prefix)
	return err
}

func (s *Store) ListRecentItems(userID string, limit int) ([]metadata.RecentItemRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, user_id, bucket, prefix, accessed_at
		FROM recent_items WHERE user_id=$1 ORDER BY accessed_at DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.RecentItemRecord
	for rows.Next() {
		var rec metadata.RecentItemRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Bucket, &rec.Prefix, &rec.AccessedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanNotifications(rows interface {
	Next() bool
	Scan(dest ...any) error
}) ([]metadata.UserNotificationRecord, error) {
	var out []metadata.UserNotificationRecord
	for rows.Next() {
		var rec metadata.UserNotificationRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Kind, &rec.Title, &rec.Body, &rec.Link, &rec.ReadAt, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}
