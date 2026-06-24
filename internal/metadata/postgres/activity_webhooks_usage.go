package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) AppendActivity(rec metadata.ActivityRecord) error {
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("%d", rec.Timestamp.UnixNano())
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO audit_logs (id, ts, username, action, resource_type, resource_name, ip_address)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO NOTHING`,
		rec.ID, rec.Timestamp, rec.User, rec.Action, rec.ResourceType, rec.ResourceName, rec.IPAddress)
	return err
}

func (s *Store) ListActivity(f metadata.ActivityFilter) (metadata.ActivityListResult, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	var since time.Time
	switch f.Period {
	case "24h":
		since = time.Now().UTC().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().UTC().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}
	q := `SELECT id, ts, COALESCE(username,''), action, COALESCE(resource_type,''), COALESCE(resource_name,''), COALESCE(ip_address,'')
		FROM audit_logs WHERE 1=1`
	var args []any
	n := 1
	if !since.IsZero() {
		q += fmt.Sprintf(` AND ts >= $%d`, n)
		args = append(args, since)
		n++
	}
	if f.LimitUser != "" {
		q += fmt.Sprintf(` AND username = $%d`, n)
		args = append(args, f.LimitUser)
		n++
	}
	if f.User != "" {
		q += fmt.Sprintf(` AND username = $%d`, n)
		args = append(args, f.User)
		n++
	}
	if f.Action != "" {
		q += fmt.Sprintf(` AND action = $%d`, n)
		args = append(args, f.Action)
		n++
	}
	if f.IP != "" {
		q += fmt.Sprintf(` AND ip_address = $%d`, n)
		args = append(args, f.IP)
		n++
	}
	q += ` ORDER BY ts DESC LIMIT 5000`
	rows, err := s.pool.Query(context.Background(), q, args...)
	if err != nil {
		return metadata.ActivityListResult{}, err
	}
	defer rows.Close()
	var all []metadata.ActivityRecord
	for rows.Next() {
		var rec metadata.ActivityRecord
		if err := rows.Scan(&rec.ID, &rec.Timestamp, &rec.User, &rec.Action, &rec.ResourceType, &rec.ResourceName, &rec.IPAddress); err != nil {
			return metadata.ActivityListResult{}, err
		}
		if f.Bucket != "" && rec.ResourceType == "bucket" && rec.ResourceName != f.Bucket {
			continue
		}
		if f.Bucket != "" && rec.ResourceType == "object" && !strings.HasPrefix(rec.ResourceName, f.Bucket+"/") {
			continue
		}
		if f.Search != "" {
			hay := strings.ToLower(rec.User + " " + rec.Action + " " + rec.ResourceName + " " + rec.ResourceType)
			if !strings.Contains(hay, strings.ToLower(f.Search)) {
				continue
			}
		}
		all = append(all, rec)
	}
	total := len(all)
	if f.Offset >= total {
		return metadata.ActivityListResult{Events: []metadata.ActivityRecord{}, Total: total}, nil
	}
	end := f.Offset + f.Limit
	if end > total {
		end = total
	}
	return metadata.ActivityListResult{Events: all[f.Offset:end], Total: total}, nil
}

func (s *Store) PutWebhook(rec metadata.WebhookRecord) error {
	events, _ := marshalJSON(rec.Events)
	headers, _ := marshalJSON(rec.Headers)
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO webhooks (id, name, url, events, headers, enabled, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET name=$2, url=$3, events=$4, headers=$5, enabled=$6`,
		rec.ID, rec.Name, rec.URL, events, headers, rec.Enabled, rec.CreatedAt)
	return err
}

func (s *Store) GetWebhook(id string) (metadata.WebhookRecord, error) {
	var rec metadata.WebhookRecord
	var events, headers []byte
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, name, url, events, headers, enabled, created_at FROM webhooks WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Name, &rec.URL, &events, &headers, &rec.Enabled, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	rec.Events = jsonStringSlice(events)
	rec.Headers = jsonMap(headers)
	return rec, nil
}

func (s *Store) ListWebhooks() ([]metadata.WebhookRecord, error) {
	rows, err := s.pool.Query(context.Background(), `SELECT id, name, url, events, headers, enabled, created_at FROM webhooks ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.WebhookRecord
	for rows.Next() {
		var rec metadata.WebhookRecord
		var events, headers []byte
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.URL, &events, &headers, &rec.Enabled, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.Events = jsonStringSlice(events)
		rec.Headers = jsonMap(headers)
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteWebhook(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM webhooks WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) PutWebhookDelivery(rec metadata.WebhookDeliveryRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO webhook_deliveries (id, webhook_id, event, url, status_code, success, error, attempts, payload, created_at, last_attempt)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (id) DO UPDATE SET status_code=$5, success=$6, error=$7, attempts=$8, last_attempt=$11`,
		rec.ID, rec.WebhookID, rec.Event, rec.URL, rec.StatusCode, rec.Success, rec.Error, rec.Attempts, rec.Payload, rec.CreatedAt, rec.LastAttempt)
	return err
}

func (s *Store) GetWebhookDelivery(id string) (metadata.WebhookDeliveryRecord, error) {
	var rec metadata.WebhookDeliveryRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, webhook_id, event, url, status_code, success, COALESCE(error,''), attempts, COALESCE(payload,''), created_at, last_attempt
		FROM webhook_deliveries WHERE id=$1`, id).Scan(
		&rec.ID, &rec.WebhookID, &rec.Event, &rec.URL, &rec.StatusCode, &rec.Success, &rec.Error, &rec.Attempts, &rec.Payload, &rec.CreatedAt, &rec.LastAttempt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) ListWebhookDeliveries(webhookID string, limit int) ([]metadata.WebhookDeliveryRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, webhook_id, event, url, status_code, success, COALESCE(error,''), attempts, COALESCE(payload,''), created_at, last_attempt
		FROM webhook_deliveries WHERE webhook_id=$1 ORDER BY created_at DESC LIMIT $2`, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.WebhookDeliveryRecord
	for rows.Next() {
		var rec metadata.WebhookDeliveryRecord
		var last pgtype.Timestamptz
		if err := rows.Scan(&rec.ID, &rec.WebhookID, &rec.Event, &rec.URL, &rec.StatusCode, &rec.Success, &rec.Error, &rec.Attempts, &rec.Payload, &rec.CreatedAt, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			rec.LastAttempt = last.Time
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) AddUsageBytes(upload, download int64) error {
	_, err := s.pool.Exec(context.Background(), `
		UPDATE usage_counters SET upload = upload + $1, download = download + $2 WHERE id='global'`, upload, download)
	return err
}

func (s *Store) GetUsageCounters() (metadata.UsageCounters, error) {
	var c metadata.UsageCounters
	err := s.pool.QueryRow(context.Background(), `SELECT upload, download FROM usage_counters WHERE id='global'`).Scan(&c.UploadBytes, &c.DownloadBytes)
	return c, err
}

func (s *Store) PutUsageSnapshot(snap metadata.UsageSnapshot) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO usage_snapshots (day, storage_bytes, object_count, bucket_count) VALUES ($1,$2,$3,$4)
		ON CONFLICT (day) DO UPDATE SET storage_bytes=$2, object_count=$3, bucket_count=$4`,
		snap.Date, snap.StorageBytes, snap.ObjectCount, snap.BucketCount)
	return err
}

func (s *Store) ListUsageSnapshots(days int) ([]metadata.UsageSnapshot, error) {
	if days <= 0 {
		days = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	rows, err := s.pool.Query(context.Background(), `
		SELECT day, storage_bytes, object_count, bucket_count FROM usage_snapshots
		WHERE day >= $1 ORDER BY day`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.UsageSnapshot
	for rows.Next() {
		var snap metadata.UsageSnapshot
		if err := rows.Scan(&snap.Date, &snap.StorageBytes, &snap.ObjectCount, &snap.BucketCount); err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) BucketUsageStats(filter metadata.BucketListFilter) ([]metadata.BucketUsage, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if filter.Unfiltered {
		rows, err = s.pool.Query(context.Background(), `
			SELECT b.name, b.owner, COALESCE(SUM(o.size),0), COUNT(o.key)
			FROM buckets b
			LEFT JOIN objects o ON o.bucket=b.storage_key AND o.is_latest=TRUE AND o.is_delete_marker=FALSE
			GROUP BY b.name, b.owner ORDER BY b.name`)
	} else {
		rows, err = s.pool.Query(context.Background(), `
			SELECT b.name, b.owner, COALESCE(SUM(o.size),0), COUNT(o.key)
			FROM buckets b
			LEFT JOIN objects o ON o.bucket=b.storage_key AND o.is_latest=TRUE AND o.is_delete_marker=FALSE
			WHERE b.owner_id = $1 OR b.owner = $2
				OR (b.team_id IS NOT NULL AND b.team_id <> '' AND b.team_id = ANY($3))
				OR (b.tenant_id IS NOT NULL AND b.tenant_id <> '' AND b.tenant_id = ANY($4))
			GROUP BY b.name, b.owner ORDER BY b.name`,
			filter.UserID, filter.Username, filter.TeamIDs, filter.TenantIDs)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.BucketUsage
	for rows.Next() {
		var u metadata.BucketUsage
		if err := rows.Scan(&u.Name, &u.Owner, &u.TotalSize, &u.ObjectCount); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) OwnerUsage(owner string) (int, int64, error) {
	var count int
	var bytes int64
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*), COALESCE(SUM(o.size),0)
		FROM objects o JOIN buckets b ON b.storage_key=o.bucket
		WHERE o.is_latest=TRUE AND o.is_delete_marker=FALSE AND b.owner=$1`, owner).Scan(&count, &bytes)
	return count, bytes, err
}
