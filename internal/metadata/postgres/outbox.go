package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

// outboxAdvisoryLock serializes cap checks inside a transaction.
const outboxAdvisoryLock int64 = 83420011

func (s *Store) maxPending() int {
	if s.outboxMaxSet {
		return s.outboxMaxPending
	}
	return events.DefaultMaxPending
}

// SetOutboxMaxPending sets the pending cap. Zero disables it.
func (s *Store) SetOutboxMaxPending(n int) {
	s.outboxMaxSet = true
	s.outboxMaxPending = n
}

func (s *Store) withOutboxTx(ctx context.Context, evs []events.OutboxRecord, mutate func(tx pgx.Tx) error) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := mutate(tx); err != nil {
		return 0, err
	}
	dropped, err := insertOutboxPg(ctx, tx, evs, s.maxPending())
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return dropped, nil
}

func (s *Store) InsertOutbox(ctx context.Context, rec events.OutboxRecord) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	n, err := insertOutboxPg(ctx, tx, []events.OutboxRecord{rec}, s.maxPending())
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return n, nil
}

func insertOutboxPg(ctx context.Context, tx pgx.Tx, evs []events.OutboxRecord, maxPending int) (int, error) {
	if len(evs) == 0 {
		return 0, nil
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, outboxAdvisoryLock); err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	dropped := 0
	for _, ev := range evs {
		ev = events.NormalizeOutbox(ev, now)
		if maxPending > 0 {
			var pending int
			if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM event_outbox WHERE status=$1`, events.StatusPending).Scan(&pending); err != nil {
				return dropped, err
			}
			for pending >= maxPending {
				var id string
				err := tx.QueryRow(ctx, `
					SELECT id FROM event_outbox
					WHERE status=$1
					ORDER BY created_at ASC, id ASC
					LIMIT 1`, events.StatusPending).Scan(&id)
				if err != nil {
					break
				}
				tag, err := tx.Exec(ctx, `
					UPDATE event_outbox
					SET status=$2, drop_reason=$3, updated_at=$4
					WHERE id=$1 AND status=$5`,
					id, events.StatusDropped, events.DropReasonCap, now, events.StatusPending)
				if err != nil {
					return dropped, err
				}
				if tag.RowsAffected() == 0 {
					break
				}
				dropped++
				pending--
			}
		}
		if err := upsertOutboxPg(ctx, tx, ev); err != nil {
			return dropped, err
		}
	}
	return dropped, nil
}

func upsertOutboxPg(ctx context.Context, tx pgx.Tx, rec events.OutboxRecord) error {
	attempts, err := json.Marshal(rec.Attempts)
	if err != nil {
		return err
	}
	if len(rec.Payload) == 0 {
		rec.Payload = []byte(`{}`)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO event_outbox (
			id, created_at, event_type, bucket, key, version_id, tenant_id, payload,
			status, attempts, next_attempt_at, drop_reason, delivered_at, dead_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,$10::jsonb,$11,$12,$13,$14,$15)
		ON CONFLICT (id) DO UPDATE SET
			event_type=EXCLUDED.event_type,
			bucket=EXCLUDED.bucket,
			key=EXCLUDED.key,
			version_id=EXCLUDED.version_id,
			tenant_id=EXCLUDED.tenant_id,
			payload=EXCLUDED.payload,
			status=EXCLUDED.status,
			attempts=EXCLUDED.attempts,
			next_attempt_at=EXCLUDED.next_attempt_at,
			drop_reason=EXCLUDED.drop_reason,
			delivered_at=EXCLUDED.delivered_at,
			dead_at=EXCLUDED.dead_at,
			updated_at=EXCLUDED.updated_at`,
		rec.ID, rec.CreatedAt, rec.EventType, rec.Bucket, rec.Key, rec.VersionID, rec.TenantID, rec.Payload,
		rec.Status, attempts, rec.NextAttemptAt, rec.DropReason, rec.DeliveredAt, rec.DeadAt, rec.UpdatedAt)
	return err
}

func (s *Store) ListDueOutbox(ctx context.Context, now time.Time, limit int) ([]events.OutboxRecord, error) {
	if limit <= 0 {
		limit = 64
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, created_at, event_type, bucket, key, version_id, tenant_id, payload,
			status, attempts, next_attempt_at, drop_reason, delivered_at, dead_at, updated_at
		FROM event_outbox
		WHERE status=$1 AND next_attempt_at <= $2
		ORDER BY next_attempt_at, id
		LIMIT $3`, events.StatusPending, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []events.OutboxRecord
	for rows.Next() {
		rec, err := scanOutbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) SaveOutbox(ctx context.Context, rec events.OutboxRecord) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := upsertOutboxPg(ctx, tx, rec); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) PurgeOutbox(ctx context.Context, status string, before time.Time) (int, error) {
	var tag interface{ RowsAffected() int64 }
	var err error
	switch status {
	case events.StatusDelivered:
		tag, err = s.pool.Exec(ctx, `DELETE FROM event_outbox WHERE status=$1 AND delivered_at IS NOT NULL AND delivered_at < $2`, status, before.UTC())
	case events.StatusDead:
		tag, err = s.pool.Exec(ctx, `DELETE FROM event_outbox WHERE status=$1 AND dead_at IS NOT NULL AND dead_at < $2`, status, before.UTC())
	case events.StatusDropped:
		tag, err = s.pool.Exec(ctx, `DELETE FROM event_outbox WHERE status=$1 AND updated_at < $2`, status, before.UTC())
	default:
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CountOutbox(ctx context.Context, status string) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM event_outbox WHERE status=$1`, status).Scan(&n)
	return n, err
}

func (s *Store) GetOutbox(ctx context.Context, id string) (events.OutboxRecord, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, created_at, event_type, bucket, key, version_id, tenant_id, payload,
			status, attempts, next_attempt_at, drop_reason, delivered_at, dead_at, updated_at
		FROM event_outbox WHERE id=$1`, id)
	rec, err := scanOutbox(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rec, metadata.ErrNotFound
		}
		return rec, err
	}
	return rec, nil
}

type outboxScanner interface {
	Scan(dest ...any) error
}

func scanOutbox(row outboxScanner) (events.OutboxRecord, error) {
	var rec events.OutboxRecord
	var payload, attempts []byte
	err := row.Scan(
		&rec.ID, &rec.CreatedAt, &rec.EventType, &rec.Bucket, &rec.Key, &rec.VersionID, &rec.TenantID, &payload,
		&rec.Status, &attempts, &rec.NextAttemptAt, &rec.DropReason, &rec.DeliveredAt, &rec.DeadAt, &rec.UpdatedAt,
	)
	if err != nil {
		return rec, err
	}
	rec.Payload = payload
	if len(attempts) > 0 {
		if err := json.Unmarshal(attempts, &rec.Attempts); err != nil {
			return rec, err
		}
	}
	if rec.Attempts == nil {
		rec.Attempts = map[string]events.SinkAttempt{}
	}
	return rec, nil
}
