package postgres

import (
	"context"
	"sort"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxReplicationErrors = 50

func (s *Store) PutReplicationTask(rec metadata.ReplicationTask) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO replication_tasks (id, rule_id, event, source_bucket, key, status, attempts, bytes, error, created_at, next_attempt, processed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET status=$6, attempts=$7, bytes=$8, error=$9, next_attempt=$11, processed_at=$12`,
		rec.ID, rec.RuleID, rec.Event, rec.SourceBucket, rec.Key, rec.Status, rec.Attempts, rec.Bytes, rec.Error,
		rec.CreatedAt, timestamptzPtr(&rec.NextAttempt), timestamptzPtr(rec.ProcessedAt))
	return err
}

func (s *Store) GetReplicationTask(id string) (metadata.ReplicationTask, error) {
	var rec metadata.ReplicationTask
	var next, processed pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, rule_id, event, source_bucket, key, status, attempts, bytes, COALESCE(error,''), created_at, next_attempt, processed_at
		FROM replication_tasks WHERE id=$1`, id).Scan(
		&rec.ID, &rec.RuleID, &rec.Event, &rec.SourceBucket, &rec.Key, &rec.Status, &rec.Attempts, &rec.Bytes, &rec.Error,
		&rec.CreatedAt, &next, &processed)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	if next.Valid {
		rec.NextAttempt = next.Time
	}
	rec.ProcessedAt = timePtr(processed)
	return rec, nil
}

func (s *Store) ListReplicationTasks(status string, limit int) ([]metadata.ReplicationTask, error) {
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	var err error
	if status == "" {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, rule_id, event, source_bucket, key, status, attempts, bytes, COALESCE(error,''), created_at, next_attempt, processed_at
			FROM replication_tasks ORDER BY created_at DESC`)
	} else {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, rule_id, event, source_bucket, key, status, attempts, bytes, COALESCE(error,''), created_at, next_attempt, processed_at
			FROM replication_tasks WHERE status=$1 ORDER BY created_at DESC`, status)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ReplicationTask
	for rows.Next() {
		rec, err := scanReplTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

func scanReplTask(rows interface{ Scan(dest ...any) error }) (metadata.ReplicationTask, error) {
	var rec metadata.ReplicationTask
	var next, processed pgtype.Timestamptz
	err := rows.Scan(&rec.ID, &rec.RuleID, &rec.Event, &rec.SourceBucket, &rec.Key, &rec.Status, &rec.Attempts, &rec.Bytes, &rec.Error,
		&rec.CreatedAt, &next, &processed)
	if err != nil {
		return rec, err
	}
	if next.Valid {
		rec.NextAttempt = next.Time
	}
	rec.ProcessedAt = timePtr(processed)
	return rec, nil
}

func (s *Store) ListDueReplicationTasks(limit int, now time.Time) ([]metadata.ReplicationTask, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, rule_id, event, source_bucket, key, status, attempts, bytes, COALESCE(error,''), created_at, next_attempt, processed_at
		FROM replication_tasks
		WHERE status=$1 AND (next_attempt IS NULL OR next_attempt <= $2)
		ORDER BY created_at LIMIT $3`, metadata.ReplTaskPending, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ReplicationTask
	for rows.Next() {
		rec, err := scanReplTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) CountPendingReplicationTasks() (int, time.Time, error) {
	var count int
	var oldest pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*), MIN(created_at) FROM replication_tasks WHERE status=$1`, metadata.ReplTaskPending).Scan(&count, &oldest)
	if err != nil {
		return 0, time.Time{}, err
	}
	if oldest.Valid {
		return count, oldest.Time, nil
	}
	return count, time.Time{}, nil
}

func (s *Store) GetGatewayStats() (metadata.GatewayStats, error) {
	var stats metadata.GatewayStats
	var oldest, last pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT pending_count, bytes_replicated, replication_errors, oldest_pending, last_processed_at, tasks_completed_total
		FROM gateway_stats WHERE id='global'`).Scan(
		&stats.PendingCount, &stats.BytesReplicated, &stats.ReplicationErrors, &oldest, &last, &stats.TasksCompletedTotal)
	if err != nil {
		return stats, err
	}
	if oldest.Valid {
		stats.OldestPending = oldest.Time
	}
	if last.Valid {
		stats.LastProcessedAt = last.Time
	}
	return stats, nil
}

func (s *Store) PutGatewayStats(stats metadata.GatewayStats) error {
	_, err := s.pool.Exec(context.Background(), `
		UPDATE gateway_stats SET pending_count=$1, bytes_replicated=$2, replication_errors=$3,
			oldest_pending=$4, last_processed_at=$5, tasks_completed_total=$6 WHERE id='global'`,
		stats.PendingCount, stats.BytesReplicated, stats.ReplicationErrors,
		timestamptzPtr(&stats.OldestPending), timestamptzPtr(&stats.LastProcessedAt), stats.TasksCompletedTotal)
	return err
}

func (s *Store) UpdateGatewayStats(fn func(*metadata.GatewayStats)) error {
	stats, err := s.GetGatewayStats()
	if err != nil {
		stats = metadata.GatewayStats{}
	}
	fn(&stats)
	return s.PutGatewayStats(stats)
}

func (s *Store) AddReplicationError(rec metadata.ReplicationErrorRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO replication_errors (id, task_id, rule_id, event, source_bucket, key, message, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, rec.ID, rec.TaskID, rec.RuleID, rec.Event, rec.SourceBucket, rec.Key, rec.Message, rec.CreatedAt)
	if err != nil {
		return err
	}
	// trim old errors
	_, _ = s.pool.Exec(context.Background(), `
		DELETE FROM replication_errors WHERE id NOT IN (
			SELECT id FROM replication_errors ORDER BY created_at DESC LIMIT $1)`, maxReplicationErrors)
	return nil
}

func (s *Store) ListReplicationErrors(limit int) ([]metadata.ReplicationErrorRecord, error) {
	if limit <= 0 {
		limit = maxReplicationErrors
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, COALESCE(task_id,''), rule_id, event, source_bucket, key, message, created_at
		FROM replication_errors ORDER BY created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ReplicationErrorRecord
	for rows.Next() {
		var rec metadata.ReplicationErrorRecord
		if err := rows.Scan(&rec.ID, &rec.TaskID, &rec.RuleID, &rec.Event, &rec.SourceBucket, &rec.Key, &rec.Message, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, rows.Err()
}

func (s *Store) ClearReplicationErrors() error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM replication_errors`)
	return err
}

func (s *Store) RetryFailedReplicationTasks() (int, error) {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE replication_tasks SET status=$1, next_attempt=NOW(), error='' WHERE status=$2`,
		metadata.ReplTaskPending, metadata.ReplTaskFailed)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) CountBrokenReplicationRules() (int, error) {
	rules, err := s.ListReplicationRules()
	if err != nil {
		return 0, err
	}
	broken := 0
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if _, err := s.GetGatewayConnection(r.DestConnection); err != nil {
			broken++
		}
	}
	return broken, nil
}
