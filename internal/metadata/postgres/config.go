package postgres

import (
	"context"
	"encoding/json"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Store) GetSystemConfig() (metadata.SystemConfig, error) {
	var cfg metadata.SystemConfig
	cfg.TrashRetentionDays = 30
	var data []byte
	err := s.pool.QueryRow(context.Background(), `SELECT data FROM system_config WHERE id='system'`).Scan(&data)
	if err != nil {
		return cfg, nil
	}
	_ = json.Unmarshal(data, &cfg)
	if cfg.TrashRetentionDays == 0 {
		cfg.TrashRetentionDays = 30
	}
	return cfg, nil
}

func (s *Store) PutSystemConfig(cfg metadata.SystemConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
		INSERT INTO system_config (id, data) VALUES ('system', $1)
		ON CONFLICT (id) DO UPDATE SET data=$1`, data)
	return err
}

func (s *Store) PutTrash(rec metadata.TrashRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO trash (id, original_bucket, original_key, trash_key, size, version_id, deleted_by, deleted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET trash_key=$4, size=$5`,
		rec.ID, rec.OriginalBucket, rec.OriginalKey, rec.TrashKey, rec.Size, rec.VersionID, rec.DeletedBy, rec.DeletedAt)
	return err
}

func (s *Store) GetTrash(id string) (metadata.TrashRecord, error) {
	var rec metadata.TrashRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, original_bucket, original_key, trash_key, size, COALESCE(version_id,''), COALESCE(deleted_by,''), deleted_at
		FROM trash WHERE id=$1`, id).Scan(
		&rec.ID, &rec.OriginalBucket, &rec.OriginalKey, &rec.TrashKey, &rec.Size, &rec.VersionID, &rec.DeletedBy, &rec.DeletedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) DeleteTrash(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM trash WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListTrash(bucketFilter string) ([]metadata.TrashRecord, error) {
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	var err error
	if bucketFilter == "" {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, original_bucket, original_key, trash_key, size, COALESCE(version_id,''), COALESCE(deleted_by,''), deleted_at
			FROM trash ORDER BY deleted_at DESC`)
	} else {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, original_bucket, original_key, trash_key, size, COALESCE(version_id,''), COALESCE(deleted_by,''), deleted_at
			FROM trash WHERE original_bucket=$1 ORDER BY deleted_at DESC`, bucketFilter)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.TrashRecord
	for rows.Next() {
		var rec metadata.TrashRecord
		if err := rows.Scan(&rec.ID, &rec.OriginalBucket, &rec.OriginalKey, &rec.TrashKey, &rec.Size,
			&rec.VersionID, &rec.DeletedBy, &rec.DeletedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) PutConsoleToken(rec metadata.ConsoleTokenRecord) error {
	scopes, _ := marshalJSON(rec.Scopes)
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO api_tokens (id, name, token_hash, user_id, username, scopes, expires_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET name=$2, token_hash=$3, scopes=$6, expires_at=$7`,
		rec.ID, rec.Name, rec.TokenHash, rec.UserID, rec.Username, scopes, rec.ExpiresAt, rec.CreatedAt)
	return err
}

func (s *Store) GetConsoleToken(id string) (metadata.ConsoleTokenRecord, error) {
	var rec metadata.ConsoleTokenRecord
	var scopes []byte
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, COALESCE(name,''), token_hash, user_id, COALESCE(username,''), scopes, expires_at, created_at
		FROM api_tokens WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Name, &rec.TokenHash, &rec.UserID, &rec.Username, &scopes, &rec.ExpiresAt, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	_ = unmarshalJSON(scopes, &rec.Scopes)
	return rec, nil
}

func (s *Store) FindConsoleTokenByHash(hash string) (metadata.ConsoleTokenRecord, error) {
	var rec metadata.ConsoleTokenRecord
	var scopes []byte
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, COALESCE(name,''), token_hash, user_id, COALESCE(username,''), scopes, expires_at, created_at
		FROM api_tokens WHERE token_hash=$1`, hash).Scan(
		&rec.ID, &rec.Name, &rec.TokenHash, &rec.UserID, &rec.Username, &scopes, &rec.ExpiresAt, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	_ = unmarshalJSON(scopes, &rec.Scopes)
	return rec, nil
}

func (s *Store) ListConsoleTokens(userID string) ([]metadata.ConsoleTokenRecord, error) {
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	var err error
	if userID == "" {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, COALESCE(name,''), token_hash, user_id, COALESCE(username,''), scopes, expires_at, created_at FROM api_tokens`)
	} else {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, COALESCE(name,''), token_hash, user_id, COALESCE(username,''), scopes, expires_at, created_at
			FROM api_tokens WHERE user_id=$1`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ConsoleTokenRecord
	for rows.Next() {
		var rec metadata.ConsoleTokenRecord
		var scopes []byte
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.TokenHash, &rec.UserID, &rec.Username, &scopes, &rec.ExpiresAt, &rec.CreatedAt); err != nil {
			return nil, err
		}
		_ = unmarshalJSON(scopes, &rec.Scopes)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteConsoleToken(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM api_tokens WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}
