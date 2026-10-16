package postgres

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) PutAccessKey(rec metadata.AccessKeyRecord) error {
	var err error
	if rec.SecretKey, err = s.fieldPrepare(fieldenc.PathAccessKeySecretKey, rec.SecretKey); err != nil {
		return err
	}
	if rec.SessionToken, err = s.fieldPrepare(fieldenc.PathAccessKeySessionToken, rec.SessionToken); err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
		INSERT INTO access_keys (access_key, secret_key, label, owner_id, owner, created_at, session_token, expires_at, created_with_mfa)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (access_key) DO UPDATE SET secret_key=$2, label=$3, owner_id=$4, owner=$5,
			session_token=$7, expires_at=$8, created_with_mfa=$9`,
		rec.AccessKey, rec.SecretKey, rec.Label, rec.OwnerID, rec.Owner, rec.CreatedAt,
		nullString(rec.SessionToken), timestamptzPtr(rec.ExpiresAt), rec.CreatedWithMFA)
	return err
}

func (s *Store) GetAccessKey(accessKey string) (metadata.AccessKeyRecord, error) {
	var rec metadata.AccessKeyRecord
	var sess *string
	var exp pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT access_key, secret_key, COALESCE(label,''), COALESCE(owner_id,''), COALESCE(owner,''), created_at,
			session_token, expires_at, created_with_mfa
		FROM access_keys WHERE access_key=$1`, accessKey).Scan(
		&rec.AccessKey, &rec.SecretKey, &rec.Label, &rec.OwnerID, &rec.Owner, &rec.CreatedAt, &sess, &exp, &rec.CreatedWithMFA)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	if sess != nil {
		rec.SessionToken = *sess
	}
	rec.ExpiresAt = timePtr(exp)
	if rec.SecretKey, err = s.fieldDecrypt(fieldenc.PathAccessKeySecretKey, rec.SecretKey); err != nil {
		return rec, err
	}
	if rec.SessionToken, err = s.fieldDecrypt(fieldenc.PathAccessKeySessionToken, rec.SessionToken); err != nil {
		return rec, err
	}
	return rec, nil
}

func (s *Store) ListAccessKeys() ([]metadata.AccessKeyRecord, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT access_key, secret_key, COALESCE(label,''), COALESCE(owner_id,''), COALESCE(owner,''), created_at, created_with_mfa, expires_at
		FROM access_keys ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.AccessKeyRecord
	for rows.Next() {
		var rec metadata.AccessKeyRecord
		var exp pgtype.Timestamptz
		if err := rows.Scan(&rec.AccessKey, &rec.SecretKey, &rec.Label, &rec.OwnerID, &rec.Owner, &rec.CreatedAt, &rec.CreatedWithMFA, &exp); err != nil {
			return nil, err
		}
		rec.ExpiresAt = timePtr(exp)
		if rec.SecretKey, err = s.fieldDecrypt(fieldenc.PathAccessKeySecretKey, rec.SecretKey); err != nil {
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

func multipartMetaArg(meta map[string]string) []byte {
	clean := map[string]string{}
	for k, v := range meta {
		k = strings.ReplaceAll(k, "\x00", "")
		v = strings.ReplaceAll(v, "\x00", "")
		if k == "" {
			continue
		}
		clean[k] = v
	}
	if len(clean) == 0 {
		return []byte("null")
	}
	raw, err := marshalJSON(clean)
	if err != nil {
		return []byte("null")
	}
	return raw
}

func (s *Store) PutMultipart(rec metadata.MultipartRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO multipart_uploads (upload_id, bucket, key, initiated, encryption, content_type, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (upload_id) DO UPDATE SET
			bucket=$2, key=$3, encryption=$5, content_type=$6, metadata=$7`,
		rec.UploadID, rec.Bucket, rec.Key, rec.Initiated, encryptionArg(rec.Encryption), rec.ContentType, multipartMetaArg(rec.Metadata))
	return err
}

func (s *Store) GetMultipart(uploadID string) (metadata.MultipartRecord, error) {
	var rec metadata.MultipartRecord
	var enc, meta []byte
	err := s.pool.QueryRow(context.Background(), `
		SELECT upload_id, bucket, key, initiated, encryption, COALESCE(content_type,''), metadata
		FROM multipart_uploads WHERE upload_id=$1`, uploadID).Scan(
		&rec.UploadID, &rec.Bucket, &rec.Key, &rec.Initiated, &enc, &rec.ContentType, &meta)
	if len(enc) > 0 {
		var oe metadata.ObjectEncryption
		if json.Unmarshal(enc, &oe) == nil {
			rec.Encryption = &oe
		}
	}
	rec.Metadata = jsonMap(meta)
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
