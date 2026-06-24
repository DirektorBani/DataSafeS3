package postgres

import (
	"context"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgtype"
)

func scanObject(row interface {
	Scan(dest ...any) error
}) (metadata.ObjectRecord, error) {
	var rec metadata.ObjectRecord
	var sched, ret pgtype.Timestamptz
	var meta, tags []byte
	err := row.Scan(&rec.Bucket, &rec.Key, &rec.VersionID, &rec.Size, &rec.ETag, &rec.ContentType,
		&rec.LastModified, &sched, &rec.IsDeleteMarker, &rec.LegalHold, &ret, &rec.StorageClass, &meta, &tags, &rec.CreatedAt)
	if err != nil {
		return rec, err
	}
	rec.ScheduledDeleteAt = timePtr(sched)
	rec.RetentionUntil = timePtr(ret)
	rec.Metadata = jsonMap(meta)
	rec.Tags = jsonMap(tags)
	return rec, nil
}

func (s *Store) PutObject(rec metadata.ObjectRecord) error {
	return s.putObject(rec, false)
}

func (s *Store) PutObjectVersioned(rec metadata.ObjectRecord) error {
	return s.putObject(rec, true)
}

func (s *Store) putObject(rec metadata.ObjectRecord, versioned bool) error {
	rec.Bucket = s.normalizeObjectBucket(rec.Bucket)
	ctx := context.Background()
	meta, _ := marshalJSON(rec.Metadata)
	tags, _ := marshalJSON(rec.Tags)
	if versioned {
		if rec.VersionID == "" {
			return metadata.ErrNotFound
		}
		_, err := s.pool.Exec(ctx, `
			UPDATE objects SET is_latest=FALSE WHERE bucket=$1 AND key=$2`,
			rec.Bucket, rec.Key)
		if err != nil {
			return err
		}
		_, err = s.pool.Exec(ctx, `
			INSERT INTO objects (bucket, key, version_id, size, etag, content_type, last_modified,
				scheduled_delete_at, is_delete_marker, legal_hold, retention_until, storage_class,
				metadata, tags, created_at, is_latest)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,TRUE)
			ON CONFLICT (bucket, key, version_id) DO UPDATE SET
				size=$4, etag=$5, content_type=$6, last_modified=$7, scheduled_delete_at=$8,
				is_delete_marker=$9, legal_hold=$10, retention_until=$11, storage_class=$12,
				metadata=$13, tags=$14, is_latest=TRUE`,
			rec.Bucket, rec.Key, rec.VersionID, rec.Size, rec.ETag, rec.ContentType, rec.LastModified,
			timestamptzPtr(rec.ScheduledDeleteAt), rec.IsDeleteMarker, rec.LegalHold,
			timestamptzPtr(rec.RetentionUntil), rec.StorageClass, meta, tags, rec.CreatedAt)
		return err
	}
	_, err := s.pool.Exec(ctx, `DELETE FROM objects WHERE bucket=$1 AND key=$2`, rec.Bucket, rec.Key)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO objects (bucket, key, version_id, size, etag, content_type, last_modified,
			scheduled_delete_at, is_delete_marker, legal_hold, retention_until, storage_class,
			metadata, tags, created_at, is_latest)
		VALUES ($1,$2,'', $3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,TRUE)`,
		rec.Bucket, rec.Key, rec.Size, rec.ETag, rec.ContentType, rec.LastModified,
		timestamptzPtr(rec.ScheduledDeleteAt), rec.IsDeleteMarker, rec.LegalHold,
		timestamptzPtr(rec.RetentionUntil), rec.StorageClass, meta, tags, rec.CreatedAt)
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
				COALESCE(storage_class,''), metadata, tags, created_at
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
			COALESCE(storage_class,''), metadata, tags, created_at
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
	bucket = s.normalizeObjectBucket(bucket)
	ctx := context.Background()
	if versioningEnabled {
		rec := metadata.ObjectRecord{
			Bucket: bucket, Key: key, VersionID: versionID,
			IsDeleteMarker: true, LastModified: time.Now().UTC(),
		}
		if versionID == "" {
			rec.VersionID = newVersionID()
		}
		return s.PutObjectVersioned(rec)
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM objects WHERE bucket=$1 AND key=$2`, bucket, key)
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
			COALESCE(storage_class,''), metadata, tags, created_at
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
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT bucket, key, version_id, size, COALESCE(etag,''), COALESCE(content_type,''),
			last_modified, scheduled_delete_at, is_delete_marker, legal_hold, retention_until,
			COALESCE(storage_class,''), metadata, tags, created_at
		FROM objects WHERE bucket=$1 AND key LIKE $2
		ORDER BY key, last_modified DESC LIMIT $3`, bucket, prefix+"%", maxKeys)
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

func (s *Store) SetObjectRetention(bucket, key, versionID string, until time.Time) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.RetentionUntil = &until
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
