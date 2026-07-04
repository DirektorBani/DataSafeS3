package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) TryAcquireLeader(ctx context.Context, holderID string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	exp := now.Add(ttl)
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO ha_leader_lock (lock_id, holder_id, acquired_at, expires_at)
		VALUES ('storage-server', $1, $2, $3)
		ON CONFLICT (lock_id) DO UPDATE SET
			holder_id = EXCLUDED.holder_id,
			acquired_at = EXCLUDED.acquired_at,
			expires_at = EXCLUDED.expires_at
		WHERE ha_leader_lock.expires_at < $2 OR ha_leader_lock.holder_id = $1`,
		holderID, now, exp)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) RenewLeader(ctx context.Context, holderID string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	exp := now.Add(ttl)
	tag, err := s.pool.Exec(ctx, `
		UPDATE ha_leader_lock SET expires_at = $3, acquired_at = $2
		WHERE lock_id = 'storage-server' AND holder_id = $1`,
		holderID, now, exp)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ReleaseLeader(ctx context.Context, holderID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM ha_leader_lock WHERE lock_id = 'storage-server' AND holder_id = $1`, holderID)
	return err
}

func (s *Store) CurrentLeader(ctx context.Context) (string, time.Time, bool, error) {
	var holder string
	var exp time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT holder_id, expires_at FROM ha_leader_lock WHERE lock_id = 'storage-server'`).
		Scan(&holder, &exp)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", time.Time{}, false, nil
		}
		return "", time.Time{}, false, err
	}
	if time.Now().UTC().After(exp) {
		return holder, exp, false, nil
	}
	return holder, exp, true, nil
}
