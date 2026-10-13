package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

func (s *Store) PutSharingPolicy(rec metadata.SharingPolicy) error {
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO sharing_policies (
			scope, scope_id, public_access_blocked, share_links_allowed, presign_allowed,
			max_share_ttl_sec, max_presign_ttl_sec, share_password_required, updated_at, updated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (scope, scope_id) DO UPDATE SET
			public_access_blocked=$3, share_links_allowed=$4, presign_allowed=$5,
			max_share_ttl_sec=$6, max_presign_ttl_sec=$7, share_password_required=$8,
			updated_at=$9, updated_by=$10`,
		rec.Scope, rec.ScopeID, rec.PublicAccessBlocked, rec.ShareLinksAllowed, rec.PresignAllowed,
		rec.MaxShareTTLSec, rec.MaxPresignTTLSec, rec.SharePasswordRequired, rec.UpdatedAt, rec.UpdatedBy)
	return err
}

func (s *Store) GetSharingPolicy(scope, scopeID string) (metadata.SharingPolicy, error) {
	var rec metadata.SharingPolicy
	err := s.pool.QueryRow(context.Background(), `
		SELECT scope, scope_id, public_access_blocked, share_links_allowed, presign_allowed,
			max_share_ttl_sec, max_presign_ttl_sec, share_password_required, updated_at, updated_by
		FROM sharing_policies WHERE scope=$1 AND scope_id=$2`, scope, scopeID).Scan(
		&rec.Scope, &rec.ScopeID, &rec.PublicAccessBlocked, &rec.ShareLinksAllowed, &rec.PresignAllowed,
		&rec.MaxShareTTLSec, &rec.MaxPresignTTLSec, &rec.SharePasswordRequired, &rec.UpdatedAt, &rec.UpdatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return rec, metadata.ErrNotFound
	}
	return rec, err
}

func (s *Store) TouchSharePasswordFailure(id string, failures int, at *time.Time) error {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE shared_links SET password_failures=$2, password_failure_at=$3 WHERE id=$1`, id, failures, at)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ClaimShareTerminalEvent(id, kind string, at time.Time) (bool, error) {
	column := ""
	switch kind {
	case "limit":
		column = "limit_event_at"
	case "expired":
		column = "expired_event_at"
	default:
		return false, errors.New("unknown share terminal event")
	}
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE shared_links SET `+column+` = $2 WHERE id = $1 AND `+column+` IS NULL`, id, at)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
