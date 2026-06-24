package postgres

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

func (s *Store) PutSharedLink(rec metadata.SharedLinkRecord) error {
	ctx := context.Background()
	_, err := s.pool.Exec(ctx, `
		INSERT INTO shared_links (id, bucket, key, token, expires_at, max_downloads, download_count, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		rec.ID, rec.Bucket, rec.Key, rec.Token, rec.ExpiresAt, rec.MaxDownloads, rec.DownloadCount, rec.CreatedBy, rec.CreatedAt)
	return err
}

func (s *Store) GetSharedLink(id string) (metadata.SharedLinkRecord, error) {
	return s.scanSharedLink(`SELECT id, bucket, key, token, expires_at, max_downloads, download_count, created_by, created_at
		FROM shared_links WHERE id=$1`, id)
}

func (s *Store) GetSharedLinkByToken(token string) (metadata.SharedLinkRecord, error) {
	return s.scanSharedLink(`SELECT id, bucket, key, token, expires_at, max_downloads, download_count, created_by, created_at
		FROM shared_links WHERE token=$1`, token)
}

func (s *Store) scanSharedLink(query string, arg any) (metadata.SharedLinkRecord, error) {
	var rec metadata.SharedLinkRecord
	err := s.pool.QueryRow(context.Background(), query, arg).Scan(
		&rec.ID, &rec.Bucket, &rec.Key, &rec.Token, &rec.ExpiresAt,
		&rec.MaxDownloads, &rec.DownloadCount, &rec.CreatedBy, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, metadata.ErrNotFound
	}
	return rec, err
}

func (s *Store) ListSharedLinks(bucket, key string) ([]metadata.SharedLinkRecord, error) {
	ctx := context.Background()
	q := `SELECT id, bucket, key, token, expires_at, max_downloads, download_count, created_by, created_at FROM shared_links WHERE 1=1`
	args := []any{}
	n := 1
	if bucket != "" {
		q += ` AND bucket=$` + strconv.Itoa(n)
		args = append(args, bucket)
		n++
	}
	if key != "" {
		q += ` AND key=$` + strconv.Itoa(n)
		args = append(args, key)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.SharedLinkRecord
	for rows.Next() {
		var rec metadata.SharedLinkRecord
		if err := rows.Scan(&rec.ID, &rec.Bucket, &rec.Key, &rec.Token, &rec.ExpiresAt,
			&rec.MaxDownloads, &rec.DownloadCount, &rec.CreatedBy, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSharedLink(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM shared_links WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) IncrementSharedLinkDownload(id string) (metadata.SharedLinkRecord, error) {
	ctx := context.Background()
	var rec metadata.SharedLinkRecord
	err := s.pool.QueryRow(ctx, `
		UPDATE shared_links SET download_count = download_count + 1
		WHERE id = $1
		  AND (expires_at IS NULL OR expires_at > NOW())
		  AND (max_downloads = 0 OR download_count < max_downloads)
		RETURNING id, bucket, key, token, expires_at, max_downloads, download_count, created_by, created_at`, id).Scan(
		&rec.ID, &rec.Bucket, &rec.Key, &rec.Token, &rec.ExpiresAt,
		&rec.MaxDownloads, &rec.DownloadCount, &rec.CreatedBy, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := s.GetSharedLink(id)
		if gerr != nil {
			return rec, gerr
		}
		if aerr := existing.Active(time.Now().UTC()); aerr != nil {
			return rec, aerr
		}
		return rec, metadata.ErrShareLimitReached
	}
	return rec, err
}

func (s *Store) PutTenantMember(rec metadata.TenantMemberRecord) error {
	if rec.Role == "" {
		rec.Role = metadata.TenantRoleMember
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO tenant_members (tenant_id, user_id, role) VALUES ($1,$2,$3)`, rec.TenantID, rec.UserID, rec.Role)
	return err
}

func (s *Store) GetTenantMember(tenantID, userID string) (metadata.TenantMemberRecord, error) {
	var rec metadata.TenantMemberRecord
	err := s.pool.QueryRow(context.Background(),
		`SELECT tenant_id, user_id, role FROM tenant_members WHERE tenant_id=$1 AND user_id=$2`, tenantID, userID).
		Scan(&rec.TenantID, &rec.UserID, &rec.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, metadata.ErrNotFound
	}
	return rec, err
}

func (s *Store) ListTenantMembers(tenantID string) ([]metadata.TenantMemberRecord, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT tenant_id, user_id, role FROM tenant_members WHERE tenant_id=$1 ORDER BY user_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenantMembers(rows)
}

func (s *Store) ListUserTenants(userID string) ([]metadata.TenantMemberRecord, error) {
	rows, err := s.pool.Query(context.Background(),
		`SELECT tenant_id, user_id, role FROM tenant_members WHERE user_id=$1 ORDER BY tenant_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenantMembers(rows)
}

func scanTenantMembers(rows pgx.Rows) ([]metadata.TenantMemberRecord, error) {
	var out []metadata.TenantMemberRecord
	for rows.Next() {
		var rec metadata.TenantMemberRecord
		if err := rows.Scan(&rec.TenantID, &rec.UserID, &rec.Role); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTenantMemberRole(tenantID, userID, role string) error {
	tag, err := s.pool.Exec(context.Background(),
		`UPDATE tenant_members SET role=$3 WHERE tenant_id=$1 AND user_id=$2`, tenantID, userID, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTenantMember(tenantID, userID string) error {
	tag, err := s.pool.Exec(context.Background(),
		`DELETE FROM tenant_members WHERE tenant_id=$1 AND user_id=$2`, tenantID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}
