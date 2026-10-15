package postgres

import (
	"context"
	"strings"
	"time"

	"encoding/json"
	"github.com/DirektorBani/datasafe/internal/events"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func scanObject(row interface {
	Scan(dest ...any) error
}) (metadata.ObjectRecord, error) {
	var rec metadata.ObjectRecord
	var sched, ret pgtype.Timestamptz
	var meta, tags, enc []byte
	err := row.Scan(&rec.Bucket, &rec.Key, &rec.VersionID, &rec.Size, &rec.ETag, &rec.ContentType,
		&rec.LastModified, &sched, &rec.IsDeleteMarker, &rec.LegalHold, &ret, &rec.StorageClass, &meta, &tags, &rec.CreatedAt, &enc)
	if err != nil {
		return rec, err
	}
	rec.ScheduledDeleteAt = timePtr(sched)
	rec.RetentionUntil = timePtr(ret)
	rec.Metadata = jsonMap(meta)
	if rec.Metadata != nil {
		if mode := rec.Metadata[retentionModeMeta]; mode != "" {
			rec.RetentionMode = mode
		}
		delete(rec.Metadata, retentionModeMeta)
		if len(rec.Metadata) == 0 {
			rec.Metadata = nil
		}
	}
	rec.Tags = jsonMap(tags)
	if len(enc) > 0 {
		var oe metadata.ObjectEncryption
		if json.Unmarshal(enc, &oe) == nil {
			rec.Encryption = &oe
		}
	}
	return rec, nil
}

func encryptionArg(e *metadata.ObjectEncryption) any {
	if e == nil {
		return nil
	}
	b, err := json.Marshal(e)
	if err != nil {
		return nil
	}
	return b
}

// SetObjectEncryptionIfAbsent writes enc when encryption IS NULL.
// A marker that is already stored is not replaced.
func (s *Store) SetObjectEncryptionIfAbsent(bucket, key, versionID string, enc *metadata.ObjectEncryption) (bool, error) {
	bucket = s.normalizeObjectBucket(bucket)
	ctx := context.Background()
	tag, err := s.pool.Exec(ctx, `
		UPDATE objects SET encryption=$1
		WHERE bucket=$2 AND key=$3
		  AND (($4 <> '' AND version_id=$4) OR ($4 = '' AND is_latest=TRUE))
		  AND encryption IS NULL`,
		encryptionArg(enc), bucket, key, versionID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) PutObject(rec metadata.ObjectRecord) error {
	_, err := s.putObject(rec, false, nil)
	return err
}

func (s *Store) PutObjectVersioned(rec metadata.ObjectRecord) error {
	_, err := s.putObject(rec, true, nil)
	return err
}

func (s *Store) PutObjectWithOutbox(rec metadata.ObjectRecord, evs []events.OutboxRecord) (int, error) {
	return s.putObject(rec, false, evs)
}

func (s *Store) PutObjectVersionedWithOutbox(rec metadata.ObjectRecord, evs []events.OutboxRecord) (int, error) {
	return s.putObject(rec, true, evs)
}

func (s *Store) putObject(rec metadata.ObjectRecord, versioned bool, evs []events.OutboxRecord) (int, error) {
	rec.Bucket = s.normalizeObjectBucket(rec.Bucket)
	ctx := context.Background()
	if len(evs) == 0 {
		return 0, s.putObjectQ(ctx, s.pool, rec, versioned)
	}
	return s.withOutboxTx(ctx, evs, func(tx pgx.Tx) error {
		return s.putObjectQ(ctx, tx, rec, versioned)
	})
}

type pgExec interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// retentionModeMeta is stored inside the metadata jsonb document. PostgreSQL
// rejects U+0000 in json text (22P05), so the private key is U+0001.
const retentionModeMeta = "\x01retention_mode"

func objectMetaArg(rec metadata.ObjectRecord) []byte {
	meta := map[string]string{}
	for k, v := range rec.Metadata {
		k = strings.ReplaceAll(k, "\x00", "")
		v = strings.ReplaceAll(v, "\x00", "")
		if k == "" || k == retentionModeMeta {
			continue
		}
		meta[k] = v
	}
	if rec.RetentionMode != "" {
		meta[retentionModeMeta] = rec.RetentionMode
	}
	if len(meta) == 0 {
		return []byte("null")
	}
	raw, _ := marshalJSON(meta)
	return raw
}

func (s *Store) putObjectQ(ctx context.Context, q pgExec, rec metadata.ObjectRecord, versioned bool) error {
	meta := objectMetaArg(rec)
	tags, _ := marshalJSON(rec.Tags)
	if versioned {
		if rec.VersionID == "" {
			return metadata.ErrNotFound
		}
		var exists bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM objects WHERE bucket=$1 AND key=$2 AND version_id=$3)`,
			rec.Bucket, rec.Key, rec.VersionID).Scan(&exists); err != nil {
			return err
		}
		// A new version becomes latest. Rewriting an older version for
		// retention or legal hold keeps the current latest row.
		if !exists {
			if _, err := q.Exec(ctx, `UPDATE objects SET is_latest=FALSE WHERE bucket=$1 AND key=$2`, rec.Bucket, rec.Key); err != nil {
				return err
			}
		}
		_, err := q.Exec(ctx, `
			INSERT INTO objects (bucket, key, version_id, size, etag, content_type, last_modified,
				scheduled_delete_at, is_delete_marker, legal_hold, retention_until, storage_class,
				metadata, tags, created_at, is_latest, encryption)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,TRUE,$16)
			ON CONFLICT (bucket, key, version_id) DO UPDATE SET
				size=$4, etag=$5, content_type=$6, last_modified=$7, scheduled_delete_at=$8,
				is_delete_marker=$9, legal_hold=$10, retention_until=$11, storage_class=$12,
				metadata=$13, tags=$14, is_latest=objects.is_latest, encryption=$16`,
			rec.Bucket, rec.Key, rec.VersionID, rec.Size, rec.ETag, rec.ContentType, rec.LastModified,
			timestamptzPtr(rec.ScheduledDeleteAt), rec.IsDeleteMarker, rec.LegalHold,
			timestamptzPtr(rec.RetentionUntil), rec.StorageClass, meta, tags, rec.CreatedAt, encryptionArg(rec.Encryption))
		return err
	}
	_, err := q.Exec(ctx, `DELETE FROM objects WHERE bucket=$1 AND key=$2`, rec.Bucket, rec.Key)
	if err != nil {
		return err
	}
	// Last commit wins. A plain INSERT races with another writer of the same
	// unversioned key (23505 objects_pkey). ON CONFLICT updates that row.
	_, err = q.Exec(ctx, `
		INSERT INTO objects (bucket, key, version_id, size, etag, content_type, last_modified,
			scheduled_delete_at, is_delete_marker, legal_hold, retention_until, storage_class,
			metadata, tags, created_at, is_latest, encryption)
		VALUES ($1,$2,'', $3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,TRUE,$15)
		ON CONFLICT (bucket, key, version_id) DO UPDATE SET
			size = EXCLUDED.size,
			etag = EXCLUDED.etag,
			content_type = EXCLUDED.content_type,
			last_modified = EXCLUDED.last_modified,
			scheduled_delete_at = EXCLUDED.scheduled_delete_at,
			is_delete_marker = EXCLUDED.is_delete_marker,
			legal_hold = EXCLUDED.legal_hold,
			retention_until = EXCLUDED.retention_until,
			storage_class = EXCLUDED.storage_class,
			metadata = EXCLUDED.metadata,
			tags = EXCLUDED.tags,
			created_at = EXCLUDED.created_at,
			is_latest = TRUE,
			encryption = EXCLUDED.encryption`,
		rec.Bucket, rec.Key, rec.Size, rec.ETag, rec.ContentType, rec.LastModified,
		timestamptzPtr(rec.ScheduledDeleteAt), rec.IsDeleteMarker, rec.LegalHold,
		timestamptzPtr(rec.RetentionUntil), rec.StorageClass, meta, tags, rec.CreatedAt, encryptionArg(rec.Encryption))
	return err
}

func (s *Store) normalizeObjectBucket(bucket string) string {
	if rec, err := s.GetBucket(bucket); err == nil {
		return rec.EffectiveStorageKey()
	}
	return bucket
}

func (s *Store) GetObject(bucket, key string) (metadata.ObjectRecord, error) {
	return s.GetObjectVersion(bucket, key, "")
}

func (s *Store) GetObjectVersion(bucket, key, versionID string) (metadata.ObjectRecord, error) {
	bucket = s.normalizeObjectBucket(bucket)
	ctx := context.Background()
	if versionID == "" {
		row := s.pool.QueryRow(ctx, `
			SELECT bucket, key, version_id, size, COALESCE(etag,''), COALESCE(content_type,''),
				last_modified, scheduled_delete_at, is_delete_marker, legal_hold, retention_until,
				COALESCE(storage_class,''), metadata, tags, created_at, encryption
			FROM objects WHERE bucket=$1 AND key=$2 AND is_latest=TRUE`, bucket, key)
		rec, err := scanObject(row)
		if err != nil {
			return rec, metadata.ErrNotFound
		}
		if rec.IsDeleteMarker {
			return rec, metadata.ErrNotFound
		}
		return rec, nil
	}
	row := s.pool.QueryRow(ctx, `
		SELECT bucket, key, version_id, size, COALESCE(etag,''), COALESCE(content_type,''),
			last_modified, scheduled_delete_at, is_delete_marker, legal_hold, retention_until,
			COALESCE(storage_class,''), metadata, tags, created_at, encryption
		FROM objects WHERE bucket=$1 AND key=$2 AND version_id=$3`, bucket, key, versionID)
	rec, err := scanObject(row)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) DeleteObject(bucket, key string) error {
	return s.DeleteObjectVersion(bucket, key, "", false)
}

func (s *Store) DeleteObjectVersion(bucket, key, versionID string, versioningEnabled bool) error {
	_, err := s.deleteObjectVersion(bucket, key, versionID, versioningEnabled, nil)
	return err
}

func (s *Store) DeleteObjectVersionWithOutbox(bucket, key, versionID string, versioningEnabled bool, evs []events.OutboxRecord) (int, error) {
	return s.deleteObjectVersion(bucket, key, versionID, versioningEnabled, evs)
}

func (s *Store) deleteObjectVersion(bucket, key, versionID string, versioningEnabled bool, evs []events.OutboxRecord) (int, error) {
	bucket = s.normalizeObjectBucket(bucket)
	ctx := context.Background()
	run := func(q pgExec) error {
		return s.deleteObjectVersionQ(ctx, q, bucket, key, versionID, versioningEnabled)
	}
	if len(evs) == 0 {
		return 0, run(s.pool)
	}
	return s.withOutboxTx(ctx, evs, func(tx pgx.Tx) error { return run(tx) })
}

func (s *Store) deleteObjectVersionQ(ctx context.Context, q pgExec, bucket, key, versionID string, versioningEnabled bool) error {
	if versioningEnabled {
		rec := metadata.ObjectRecord{
			Bucket: bucket, Key: key, VersionID: versionID,
			IsDeleteMarker: true, LastModified: time.Now().UTC(),
		}
		if versionID == "" {
			rec.VersionID = newVersionID()
		}
		return s.putObjectQ(ctx, q, rec, true)
	}
	tag, err := q.Exec(ctx, `DELETE FROM objects WHERE bucket=$1 AND key=$2`, bucket, key)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func newVersionID() string {
	return time.Now().UTC().Format("20060102150405.000")
}

func (s *Store) ListObjects(bucket, prefix string, maxKeys int) ([]metadata.ObjectRecord, error) {
	objs, _, _, err := s.ListObjectsPage(bucket, prefix, "", maxKeys)
	return objs, err
}

func (s *Store) ListObjectsPage(bucket, prefix, startAfter string, maxKeys int) ([]metadata.ObjectRecord, bool, string, error) {
	bucket = s.normalizeObjectBucket(bucket)
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	ctx := context.Background()
	rows, err := s.readQueryPool().Query(ctx, `
		SELECT bucket, key, version_id, size, COALESCE(etag,''), COALESCE(content_type,''),
			last_modified, scheduled_delete_at, is_delete_marker, legal_hold, retention_until,
			COALESCE(storage_class,''), metadata, tags, created_at, encryption
		FROM objects WHERE bucket=$1 AND is_latest=TRUE AND is_delete_marker=FALSE
			AND key LIKE $2 AND ($3 = '' OR key > $3)
		ORDER BY key LIMIT $4`, bucket, prefix+"%", startAfter, maxKeys+1)
	if err != nil {
		return nil, false, "", metadata.ErrNotFound
	}
	defer rows.Close()
	var out []metadata.ObjectRecord
	for rows.Next() {
		rec, err := scanObject(rows)
		if err != nil {
			return nil, false, "", err
		}
		out = append(out, rec)
	}
	truncated := len(out) > maxKeys
	if truncated {
		out = out[:maxKeys]
	}
	next := ""
	if truncated && len(out) > 0 {
		next = out[len(out)-1].Key
	}
	return out, truncated, next, nil
}

func (s *Store) ListObjectVersions(bucket, prefix string, maxKeys int) ([]metadata.ObjectRecord, error) {
	q := `
		SELECT bucket, key, version_id, size, COALESCE(etag,''), COALESCE(content_type,''),
			last_modified, scheduled_delete_at, is_delete_marker, legal_hold, retention_until,
			COALESCE(storage_class,''), metadata, tags, created_at, encryption
		FROM objects WHERE bucket=$1 AND key LIKE $2
		ORDER BY key, last_modified DESC, version_id DESC`
	args := []any{bucket, prefix + "%"}
	if maxKeys > 0 {
		q += ` LIMIT $3`
		args = append(args, maxKeys)
	}
	rows, err := s.pool.Query(context.Background(), q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ObjectRecord
	for rows.Next() {
		rec, err := scanObject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) ListObjectVersionIDs(bucket, key string) ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT version_id FROM objects WHERE bucket=$1 AND key=$2 AND version_id != ''
		ORDER BY last_modified DESC`, bucket, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) SetObjectTags(bucket, key, versionID string, tags map[string]string) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.Tags = tags
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}

func (s *Store) UpdateObjectMeta(bucket, key, versionID string, meta map[string]string, contentType string) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.Metadata = meta
	if contentType != "" {
		rec.ContentType = contentType
	}
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}

func (s *Store) SetObjectLegalHold(bucket, key, versionID string, hold bool) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.LegalHold = hold
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}

func (s *Store) SetObjectRetention(bucket, key, versionID string, until time.Time, mode string) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.RetentionUntil = &until
	if mode = strings.ToUpper(strings.TrimSpace(mode)); mode != "" {
		rec.RetentionMode = mode
	}
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}

func (s *Store) TotalObjectBytes() (int64, error) {
	var total int64
	err := s.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(size),0) FROM objects WHERE is_latest=TRUE AND is_delete_marker=FALSE`).Scan(&total)
	return total, err
}

func (s *Store) CountObjects() (int, error) {
	var n int
	err := s.readQueryPool().QueryRow(context.Background(), `
		SELECT COUNT(*) FROM objects WHERE is_latest=TRUE AND is_delete_marker=FALSE`).Scan(&n)
	return n, err
}

func (s *Store) BucketObjectCount(bucket string) (int, error) {
	var n int
	err := s.readQueryPool().QueryRow(context.Background(), `
		SELECT COUNT(*) FROM objects WHERE bucket=$1 AND is_latest=TRUE AND is_delete_marker=FALSE`, bucket).Scan(&n)
	return n, err
}

func (s *Store) BucketTotalSize(bucket string) (int64, error) {
	var total int64
	err := s.readQueryPool().QueryRow(context.Background(), `
		SELECT COALESCE(SUM(size),0) FROM objects WHERE bucket=$1 AND is_latest=TRUE AND is_delete_marker=FALSE`, bucket).Scan(&total)
	return total, err
}
