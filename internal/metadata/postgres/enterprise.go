package postgres

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/security/fieldenc"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) EnsureDefaultTenant() error {
	_, err := s.GetTenant(metadata.DefaultTenantID)
	if err == nil {
		return nil
	}
	return s.PutTenant(metadata.TenantRecord{
		ID: metadata.DefaultTenantID, Name: "Default", Status: metadata.StatusActive, CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) PutTenant(rec metadata.TenantRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO tenants (id, name, status, created_at) VALUES ($1,$2,$3,$4)
		ON CONFLICT (id) DO UPDATE SET name=$2, status=$3`, rec.ID, rec.Name, rec.Status, rec.CreatedAt)
	return err
}

func (s *Store) GetTenant(id string) (metadata.TenantRecord, error) {
	var rec metadata.TenantRecord
	err := s.pool.QueryRow(context.Background(), `SELECT id, name, status, created_at FROM tenants WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Name, &rec.Status, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) ListTenants() ([]metadata.TenantRecord, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT id, name, status, created_at FROM tenants ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.TenantRecord
	for rows.Next() {
		var rec metadata.TenantRecord
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Status, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTenant(id string) error {
	if id == metadata.DefaultTenantID {
		return errors.New("cannot delete default tenant")
	}
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM tenants WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) PutGatewayConnection(rec metadata.GatewayConnection) error {
	var err error
	if rec.AccessKey, err = s.fieldPrepare(fieldenc.PathGatewayAccessKey, rec.AccessKey); err != nil {
		return err
	}
	if rec.SecretKey, err = s.fieldPrepare(fieldenc.PathGatewaySecretKey, rec.SecretKey); err != nil {
		return err
	}
	_, err = s.pool.Exec(context.Background(), `
		INSERT INTO gateway_connections (id, name, endpoint, region, access_key, secret_key, path_style, tls_verify, status, last_check, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET name=$2, endpoint=$3, region=$4, access_key=$5, secret_key=$6, path_style=$7, tls_verify=$8, status=$9, last_check=$10`,
		rec.ID, rec.Name, rec.Endpoint, rec.Region, rec.AccessKey, rec.SecretKey, rec.PathStyle, rec.TLSVerify, rec.Status, timestamptzPtr(&rec.LastCheck), rec.CreatedAt)
	return err
}

func (s *Store) GetGatewayConnection(id string) (metadata.GatewayConnection, error) {
	var rec metadata.GatewayConnection
	var last pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, name, endpoint, COALESCE(region,''), access_key, secret_key, path_style, tls_verify,
			COALESCE(status,''), last_check, created_at FROM gateway_connections WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Name, &rec.Endpoint, &rec.Region, &rec.AccessKey, &rec.SecretKey, &rec.PathStyle, &rec.TLSVerify,
		&rec.Status, &last, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	if last.Valid {
		rec.LastCheck = last.Time
	}
	if rec.AccessKey, err = s.fieldDecrypt(fieldenc.PathGatewayAccessKey, rec.AccessKey); err != nil {
		return rec, err
	}
	if rec.SecretKey, err = s.fieldDecrypt(fieldenc.PathGatewaySecretKey, rec.SecretKey); err != nil {
		return rec, err
	}
	return rec, nil
}

func (s *Store) ListGatewayConnections() ([]metadata.GatewayConnection, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, name, endpoint, COALESCE(region,''), access_key, secret_key, path_style, tls_verify,
			COALESCE(status,''), last_check, created_at FROM gateway_connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.GatewayConnection
	for rows.Next() {
		var rec metadata.GatewayConnection
		var last pgtype.Timestamptz
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Endpoint, &rec.Region, &rec.AccessKey, &rec.SecretKey, &rec.PathStyle, &rec.TLSVerify,
			&rec.Status, &last, &rec.CreatedAt); err != nil {
			return nil, err
		}
		if last.Valid {
			rec.LastCheck = last.Time
		}
		if rec.AccessKey, err = s.fieldDecrypt(fieldenc.PathGatewayAccessKey, rec.AccessKey); err != nil {
			return nil, err
		}
		if rec.SecretKey, err = s.fieldDecrypt(fieldenc.PathGatewaySecretKey, rec.SecretKey); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteGatewayConnection(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM gateway_connections WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) PutReplicationRule(rec metadata.ReplicationRule) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO replication_rules (id, name, source_bucket, dest_connection_id, dest_bucket, enabled, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET name=$2, source_bucket=$3, dest_connection_id=$4, dest_bucket=$5, enabled=$6`,
		rec.ID, rec.Name, rec.SourceBucket, rec.DestConnection, rec.DestBucket, rec.Enabled, rec.CreatedAt)
	return err
}

func (s *Store) GetReplicationRule(id string) (metadata.ReplicationRule, error) {
	var rec metadata.ReplicationRule
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, name, source_bucket, dest_connection_id, dest_bucket, enabled, created_at
		FROM replication_rules WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Name, &rec.SourceBucket, &rec.DestConnection, &rec.DestBucket, &rec.Enabled, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) ListReplicationRules() ([]metadata.ReplicationRule, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, name, source_bucket, dest_connection_id, dest_bucket, enabled, created_at FROM replication_rules`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ReplicationRule
	for rows.Next() {
		var rec metadata.ReplicationRule
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.SourceBucket, &rec.DestConnection, &rec.DestBucket, &rec.Enabled, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteReplicationRule(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM replication_rules WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListReplicationRulesForBucket(bucket string) ([]metadata.ReplicationRule, error) {
	all, err := s.ListReplicationRules()
	if err != nil {
		return nil, err
	}
	var out []metadata.ReplicationRule
	for _, r := range all {
		if r.SourceBucket == bucket && r.Enabled {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) PutSyncJob(rec metadata.SyncJob) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO sync_jobs (id, rule_id, status, objects_synced, errors, message, started_at, ended_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET status=$3, objects_synced=$4, errors=$5, message=$6, ended_at=$8`,
		rec.ID, rec.RuleID, rec.Status, rec.Objects, rec.Errors, rec.Message, rec.StartedAt, timestamptzPtr(rec.EndedAt))
	return err
}

func (s *Store) GetSyncJob(id string) (metadata.SyncJob, error) {
	var rec metadata.SyncJob
	var ended pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, rule_id, status, objects_synced, errors, COALESCE(message,''), started_at, ended_at
		FROM sync_jobs WHERE id=$1`, id).Scan(
		&rec.ID, &rec.RuleID, &rec.Status, &rec.Objects, &rec.Errors, &rec.Message, &rec.StartedAt, &ended)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	rec.EndedAt = timePtr(ended)
	return rec, nil
}

func (s *Store) ListSyncJobs(ruleID string, limit int) ([]metadata.SyncJob, error) {
	var rows interface {
		Next() bool
		Scan(dest ...any) error
		Close()
		Err() error
	}
	var err error
	if ruleID == "" {
		rows, err = s.pool.Query(context.Background(), `SELECT id, rule_id, status, objects_synced, errors, COALESCE(message,''), started_at, ended_at FROM sync_jobs ORDER BY started_at DESC`)
	} else {
		rows, err = s.pool.Query(context.Background(), `
			SELECT id, rule_id, status, objects_synced, errors, COALESCE(message,''), started_at, ended_at
			FROM sync_jobs WHERE rule_id=$1 ORDER BY started_at DESC`, ruleID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.SyncJob
	for rows.Next() {
		var rec metadata.SyncJob
		var ended pgtype.Timestamptz
		if err := rows.Scan(&rec.ID, &rec.RuleID, &rec.Status, &rec.Objects, &rec.Errors, &rec.Message, &rec.StartedAt, &ended); err != nil {
			return nil, err
		}
		rec.EndedAt = timePtr(ended)
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, rows.Err()
}

func (s *Store) PutFederationCluster(rec metadata.FederationCluster) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO federation_clusters (id, name, endpoint, region, status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO UPDATE SET name=$2, endpoint=$3, region=$4, status=$5`,
		rec.ID, rec.Name, rec.Endpoint, rec.Region, rec.Status, rec.CreatedAt)
	return err
}

func (s *Store) GetFederationCluster(id string) (metadata.FederationCluster, error) {
	var rec metadata.FederationCluster
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, name, endpoint, COALESCE(region,''), COALESCE(status,''), created_at FROM federation_clusters WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Name, &rec.Endpoint, &rec.Region, &rec.Status, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) ListFederationClusters() ([]metadata.FederationCluster, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, name, endpoint, COALESCE(region,''), COALESCE(status,''), created_at FROM federation_clusters ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.FederationCluster
	for rows.Next() {
		var rec metadata.FederationCluster
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.Endpoint, &rec.Region, &rec.Status, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteFederationCluster(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM federation_clusters WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}
