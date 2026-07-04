package postgres

import (
	"context"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) PutSiteReplicationPeer(rec metadata.SiteReplicationPeer) error {
	var err error
	if rec.SecretKey, err = s.fieldPrepare(fieldenc.PathSiteReplSecretKey, rec.SecretKey); err != nil {
		return err
	}
	ctx := context.Background()
	_, err = s.pool.Exec(ctx, `
		INSERT INTO site_replication_peers (id, name, endpoint, access_key, secret_key, enabled, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET name=$2, endpoint=$3, access_key=$4, secret_key=$5, enabled=$6`,
		rec.ID, rec.Name, rec.Endpoint, rec.AccessKey, rec.SecretKey, rec.Enabled, rec.CreatedAt)
	return err
}

func (s *Store) GetSiteReplicationPeer(id string) (metadata.SiteReplicationPeer, error) {
	var rec metadata.SiteReplicationPeer
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, name, endpoint, access_key, secret_key, enabled, created_at
		FROM site_replication_peers WHERE id=$1`, id).
		Scan(&rec.ID, &rec.Name, &rec.Endpoint, &rec.AccessKey, &rec.SecretKey, &rec.Enabled, &rec.CreatedAt)
	if err == pgx.ErrNoRows {
		return rec, metadata.ErrNotFound
	}
	if err != nil {
		return rec, err
	}
	if rec.SecretKey, err = s.fieldDecrypt(fieldenc.PathSiteReplSecretKey, rec.SecretKey); err != nil {
		return rec, err
	}
	return rec, nil
}

func (s *Store) ListSiteReplicationPeers() ([]metadata.SiteReplicationPeer, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, name, endpoint, access_key, secret_key, enabled, created_at
		FROM site_replication_peers ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.SiteReplicationPeer
	for rows.Next() {
		var rec metadata.SiteReplicationPeer
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Endpoint, &rec.AccessKey, &rec.SecretKey, &rec.Enabled, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if rec.SecretKey, err = s.fieldDecrypt(fieldenc.PathSiteReplSecretKey, rec.SecretKey); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSiteReplicationPeer(id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM site_replication_peers WHERE id=$1`, id)
	return err
}

func (s *Store) PutSiteReplicationRule(rec metadata.SiteReplicationRule) error {
	if rec.Direction == "" {
		rec.Direction = "one-way"
	}
	var peerID, trustedID *string
	if rec.PeerID != "" {
		peerID = &rec.PeerID
	}
	if rec.TrustedClusterID != "" {
		trustedID = &rec.TrustedClusterID
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO site_replication_rules (id, peer_id, trusted_cluster_id, source_bucket, dest_bucket, direction, enabled, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET peer_id=$2, trusted_cluster_id=$3, source_bucket=$4, dest_bucket=$5, direction=$6, enabled=$7`,
		rec.ID, peerID, trustedID, rec.SourceBucket, rec.DestBucket, rec.Direction, rec.Enabled, rec.CreatedAt)
	return err
}

func (s *Store) ListSiteReplicationRules() ([]metadata.SiteReplicationRule, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, COALESCE(peer_id,''), COALESCE(trusted_cluster_id,''), source_bucket, dest_bucket, direction, enabled, created_at
		FROM site_replication_rules ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.SiteReplicationRule
	for rows.Next() {
		var rec metadata.SiteReplicationRule
		if err := rows.Scan(&rec.ID, &rec.PeerID, &rec.TrustedClusterID, &rec.SourceBucket, &rec.DestBucket, &rec.Direction, &rec.Enabled, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSiteReplicationRule(id string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM site_replication_rules WHERE id=$1`, id)
	return err
}

func (s *Store) ListSiteReplicationRulesForBucket(bucket string) ([]metadata.SiteReplicationRule, error) {
	all, err := s.ListSiteReplicationRules()
	if err != nil {
		return nil, err
	}
	var out []metadata.SiteReplicationRule
	for _, r := range all {
		if r.Enabled && r.SourceBucket == bucket {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListSiteReplicationRulesForTrustedCluster(clusterID string) ([]metadata.SiteReplicationRule, error) {
	all, err := s.ListSiteReplicationRules()
	if err != nil {
		return nil, err
	}
	var out []metadata.SiteReplicationRule
	for _, r := range all {
		if r.TrustedClusterID == clusterID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) PutSiteReplicationTask(rec metadata.SiteReplicationTask) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO site_replication_tasks (id, rule_id, event, source_bucket, object_key, status, attempts, bytes, error, created_at, next_attempt, processed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (id) DO UPDATE SET status=$6, attempts=$7, bytes=$8, error=$9, next_attempt=$11, processed_at=$12`,
		rec.ID, rec.RuleID, rec.Event, rec.SourceBucket, rec.Key, rec.Status, rec.Attempts, rec.Bytes, rec.Error,
		rec.CreatedAt, timestamptzPtr(ptrTime(rec.NextAttempt)), timestamptzPtr(rec.ProcessedAt))
	return err
}

func (s *Store) ListDueSiteReplicationTasks(limit int, now time.Time) ([]metadata.SiteReplicationTask, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, rule_id, event, source_bucket, object_key, status, attempts, bytes, COALESCE(error,''), created_at, next_attempt, processed_at
		FROM site_replication_tasks
		WHERE status=$1 AND (next_attempt IS NULL OR next_attempt <= $2)
		ORDER BY created_at ASC LIMIT $3`, metadata.SiteReplTaskPending, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.SiteReplicationTask
	for rows.Next() {
		var rec metadata.SiteReplicationTask
		var next, proc pgtype.Timestamptz
		if err := rows.Scan(&rec.ID, &rec.RuleID, &rec.Event, &rec.SourceBucket, &rec.Key, &rec.Status,
			&rec.Attempts, &rec.Bytes, &rec.Error, &rec.CreatedAt, &next, &proc); err != nil {
			return nil, err
		}
		rec.NextAttempt = timeVal(next)
		rec.ProcessedAt = timePtr(proc)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) CountPendingSiteReplicationTasks() (int, error) {
	var n int
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM site_replication_tasks WHERE status=$1`, metadata.SiteReplTaskPending).Scan(&n)
	return n, err
}

func (s *Store) SiteReplicationStatus() (metadata.SiteReplicationStatus, error) {
	pending, err := s.CountPendingSiteReplicationTasks()
	if err != nil {
		return metadata.SiteReplicationStatus{}, err
	}
	var st metadata.SiteReplicationStatus
	st.PendingCount = pending
	var lastErr *string
	var lastProc pgtype.Timestamptz
	_ = s.pool.QueryRow(context.Background(), `
		SELECT error, processed_at FROM site_replication_tasks
		WHERE status IN ($1,$2) ORDER BY processed_at DESC NULLS LAST LIMIT 1`,
		metadata.SiteReplTaskFailed, metadata.SiteReplTaskDone).
		Scan(&lastErr, &lastProc)
	if lastErr != nil {
		st.LastError = *lastErr
	}
	if lastProc.Valid {
		st.LastProcessedAt = lastProc.Time
		st.LagSeconds = time.Since(lastProc.Time).Seconds()
	}
	return st, nil
}

func timeVal(t pgtype.Timestamptz) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
