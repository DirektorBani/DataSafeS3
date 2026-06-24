package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) PutAccessKey(rec metadata.AccessKeyRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO access_keys (access_key, secret_key, label, owner_id, owner, created_at, session_token, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (access_key) DO UPDATE SET secret_key=$2, label=$3, owner_id=$4, owner=$5,
			session_token=$7, expires_at=$8`,
		rec.AccessKey, rec.SecretKey, rec.Label, rec.OwnerID, rec.Owner, rec.CreatedAt,
		nullString(rec.SessionToken), timestamptzPtr(rec.ExpiresAt))
	return err
}

func (s *Store) GetAccessKey(accessKey string) (metadata.AccessKeyRecord, error) {
	var rec metadata.AccessKeyRecord
	var sess *string
	var exp pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT access_key, secret_key, COALESCE(label,''), COALESCE(owner_id,''), COALESCE(owner,''), created_at,
			session_token, expires_at
		FROM access_keys WHERE access_key=$1`, accessKey).Scan(
		&rec.AccessKey, &rec.SecretKey, &rec.Label, &rec.OwnerID, &rec.Owner, &rec.CreatedAt, &sess, &exp)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	if sess != nil {
		rec.SessionToken = *sess
	}
	rec.ExpiresAt = timePtr(exp)
	return rec, nil
}

func (s *Store) ListAccessKeys() ([]metadata.AccessKeyRecord, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT access_key, secret_key, COALESCE(label,''), COALESCE(owner_id,''), COALESCE(owner,''), created_at
		FROM access_keys ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.AccessKeyRecord
	for rows.Next() {
		var rec metadata.AccessKeyRecord
		if err := rows.Scan(&rec.AccessKey, &rec.SecretKey, &rec.Label, &rec.OwnerID, &rec.Owner, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAccessKey(accessKey string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM access_keys WHERE access_key=$1`, accessKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) PutMultipart(rec metadata.MultipartRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO multipart_uploads (upload_id, bucket, key, initiated) VALUES ($1,$2,$3,$4)
		ON CONFLICT (upload_id) DO UPDATE SET bucket=$2, key=$3`,
		rec.UploadID, rec.Bucket, rec.Key, rec.Initiated)
	return err
}

func (s *Store) GetMultipart(uploadID string) (metadata.MultipartRecord, error) {
	var rec metadata.MultipartRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT upload_id, bucket, key, initiated FROM multipart_uploads WHERE upload_id=$1`, uploadID).Scan(
		&rec.UploadID, &rec.Bucket, &rec.Key, &rec.Initiated)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) DeleteMultipart(uploadID string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM multipart_uploads WHERE upload_id=$1`, uploadID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListMultipart(bucket string) ([]metadata.MultipartRecord, error) {
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	var err error
	if bucket == "" {
		rows, err = s.pool.Query(context.Background(), `
			SELECT upload_id, bucket, key, initiated FROM multipart_uploads ORDER BY initiated`)
	} else {
		rows, err = s.pool.Query(context.Background(), `
			SELECT upload_id, bucket, key, initiated FROM multipart_uploads WHERE bucket=$1 ORDER BY initiated`, bucket)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.MultipartRecord
	for rows.Next() {
		var rec metadata.MultipartRecord
		if err := rows.Scan(&rec.UploadID, &rec.Bucket, &rec.Key, &rec.Initiated); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
