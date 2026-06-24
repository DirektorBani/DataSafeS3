package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

const bucketSelect = `
	SELECT COALESCE(storage_key, name), name, owner, COALESCE(owner_id,''), COALESCE(team_id,''), tenant_id, created_at, COALESCE(policy,''), lifecycle_rules, COALESCE(description,''),
		versioning_enabled, versioning_suspended, object_lock_enabled, retention_days, COALESCE(retention_mode,''), COALESCE(storage_class,''),
		COALESCE(visibility,''), max_size_bytes, max_objects, tags`

func scanBucketRow(row interface{ Scan(dest ...any) error }) (metadata.BucketRecord, error) {
	var rec metadata.BucketRecord
	var lifecycle, tags []byte
	err := row.Scan(&rec.StorageKey, &rec.Name, &rec.Owner, &rec.OwnerID, &rec.TeamID, &rec.TenantID, &rec.CreatedAt, &rec.Policy, &lifecycle, &rec.Description,
		&rec.Versioning, &rec.VersioningSuspended, &rec.ObjectLock, &rec.RetentionDays, &rec.RetentionMode, &rec.StorageClass,
		&rec.Visibility, &rec.MaxSizeBytes, &rec.MaxObjects, &tags)
	if err != nil {
		return rec, err
	}
	_ = unmarshalJSON(lifecycle, &rec.LifecycleRules)
	rec.Tags = jsonMap(tags)
	return rec, nil
}

func scanBucketRows(rows pgx.Rows) ([]metadata.BucketRecord, error) {
	defer rows.Close()
	var out []metadata.BucketRecord
	for rows.Next() {
		rec, err := scanBucketRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) bucketExistsInScope(ctx context.Context, scope metadata.BucketScope, name string) (bool, error) {
	var exists bool
	var err error
	switch scope.Kind {
	case metadata.ScopeTenant:
		err = s.pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM buckets
				WHERE tenant_id = $1 AND name = $2
					AND tenant_id IS NOT NULL AND tenant_id <> '' AND tenant_id <> 'default'
			)`, scope.TenantID, name).Scan(&exists)
	default:
		err = s.pool.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM buckets
				WHERE COALESCE(owner_id, '') = $1 AND name = $2
					AND (tenant_id IS NULL OR tenant_id = '' OR tenant_id = 'default')
			)`, scope.OwnerID, name).Scan(&exists)
	}
	return exists, err
}

func (s *Store) PutBucket(rec metadata.BucketRecord) error {
	scope := metadata.BucketScopeForRecord(rec.TenantID, rec.OwnerID)
	if rec.StorageKey == "" {
		rec.StorageKey = metadata.MakeStorageKey(scope, rec.Name)
	}
	ctx := context.Background()
	exists, err := s.bucketExistsInScope(ctx, scope, rec.Name)
	if err != nil {
		return err
	}
	if exists {
		return metadata.ErrBucketExists
	}
	lifecycle, _ := marshalJSON(rec.LifecycleRules)
	tags, _ := marshalJSON(rec.Tags)
	_, err = s.pool.Exec(ctx, `
		INSERT INTO buckets (storage_key, name, owner, owner_id, team_id, tenant_id, created_at, policy, lifecycle_rules, description,
			versioning_enabled, versioning_suspended, object_lock_enabled, retention_days, retention_mode, storage_class,
			visibility, max_size_bytes, max_objects, tags)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`,
		rec.StorageKey, rec.Name, rec.Owner, optionalText(rec.OwnerID), optionalText(rec.TeamID), rec.TenantID, rec.CreatedAt, rec.Policy, lifecycle, rec.Description,
		rec.Versioning, rec.VersioningSuspended, rec.ObjectLock, rec.RetentionDays, rec.RetentionMode, rec.StorageClass,
		rec.Visibility, rec.MaxSizeBytes, rec.MaxObjects, tags)
	return err
}

func (s *Store) GetBucketByKey(storageKey string) (metadata.BucketRecord, error) {
	rec, err := scanBucketRow(s.pool.QueryRow(context.Background(), bucketSelect+` FROM buckets WHERE storage_key=$1`, storageKey))
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) GetBucket(name string) (metadata.BucketRecord, error) {
	rec, err := s.GetBucketByKey(name)
	if err == nil {
		return rec, nil
	}
	all, err := s.ListBuckets()
	if err != nil {
		return rec, err
	}
	var matches []metadata.BucketRecord
	for _, b := range all {
		if b.Name == name {
			matches = append(matches, b)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return metadata.BucketRecord{}, metadata.ErrNotFound
}

func (s *Store) ResolveBucket(scope metadata.BucketScope, name string) (metadata.BucketRecord, error) {
	ctx := context.Background()
	switch scope.Kind {
	case metadata.ScopeTenant:
		rec, err := scanBucketRow(s.pool.QueryRow(ctx, bucketSelect+`
			FROM buckets
			WHERE tenant_id = $1 AND name = $2
				AND tenant_id IS NOT NULL AND tenant_id <> '' AND tenant_id <> 'default'`, scope.TenantID, name))
		if err != nil {
			return metadata.BucketRecord{}, metadata.ErrNotFound
		}
		return rec, nil
	default:
		rec, err := scanBucketRow(s.pool.QueryRow(ctx, bucketSelect+`
			FROM buckets
			WHERE COALESCE(owner_id, '') = $1 AND name = $2
				AND (tenant_id IS NULL OR tenant_id = '' OR tenant_id = 'default')`, scope.OwnerID, name))
		if err == nil {
			return rec, nil
		}
		// Legacy bucket: storage_key == name, must belong to this owner scope.
		rec, err = scanBucketRow(s.pool.QueryRow(ctx, bucketSelect+` FROM buckets WHERE storage_key=$1 AND name=$1`, name))
		if err != nil {
			return metadata.BucketRecord{}, metadata.ErrNotFound
		}
		if !rec.LegacyBucket() || rec.OwnerID != scope.OwnerID {
			return metadata.BucketRecord{}, metadata.ErrNotFound
		}
		return rec, nil
	}
}

func (s *Store) DeleteBucket(storageKey string) error {
	ctx := context.Background()
	var objCount int
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM objects WHERE bucket=$1 AND is_latest=TRUE AND is_delete_marker=FALSE`, storageKey).Scan(&objCount)
	if objCount > 0 {
		return metadata.ErrBucketNotEmpty
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM buckets WHERE storage_key=$1`, storageKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListBuckets() ([]metadata.BucketRecord, error) {
	rows, err := s.readQueryPool().Query(context.Background(), bucketSelect+` FROM buckets ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return scanBucketRows(rows)
}

func (s *Store) UpdateBucket(rec metadata.BucketRecord) error {
	lifecycle, _ := marshalJSON(rec.LifecycleRules)
	tags, _ := marshalJSON(rec.Tags)
	key := rec.EffectiveStorageKey()
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE buckets SET owner=$2, owner_id=$3, team_id=$4, tenant_id=$5, policy=$6, lifecycle_rules=$7, description=$8,
			versioning_enabled=$9, versioning_suspended=$10, object_lock_enabled=$11, retention_days=$12,
			retention_mode=$13, storage_class=$14, visibility=$15, max_size_bytes=$16, max_objects=$17, tags=$18
		WHERE storage_key=$1`,
		key, rec.Owner, optionalText(rec.OwnerID), optionalText(rec.TeamID), rec.TenantID, rec.Policy, lifecycle, rec.Description,
		rec.Versioning, rec.VersioningSuspended, rec.ObjectLock, rec.RetentionDays, rec.RetentionMode, rec.StorageClass,
		rec.Visibility, rec.MaxSizeBytes, rec.MaxObjects, tags)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) SetBucketPolicy(storageKey, policy string) error {
	tag, err := s.pool.Exec(context.Background(), `UPDATE buckets SET policy=$2 WHERE storage_key=$1`, storageKey, policy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) SetBucketLifecycle(storageKey string, rules []metadata.LifecycleRule) error {
	data, _ := marshalJSON(rules)
	tag, err := s.pool.Exec(context.Background(), `UPDATE buckets SET lifecycle_rules=$2 WHERE storage_key=$1`, storageKey, data)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) SetBucketTags(storageKey string, tags map[string]string) error {
	data, _ := marshalJSON(tags)
	tag, err := s.pool.Exec(context.Background(), `UPDATE buckets SET tags=$2 WHERE storage_key=$1`, storageKey, data)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListBucketsByTenant(tenantID string) ([]metadata.BucketRecord, error) {
	all, err := s.ListBuckets()
	if err != nil {
		return nil, err
	}
	if tenantID == "" {
		return all, nil
	}
	members, _ := s.ListTenantMembers(tenantID)
	memberOwners := make(map[string]struct{}, len(members))
	for _, m := range members {
		memberOwners[m.UserID] = struct{}{}
	}
	seen := make(map[string]struct{})
	var out []metadata.BucketRecord
	for _, b := range all {
		if !metadata.BucketBelongsToTenant(b, tenantID, memberOwners) {
			continue
		}
		key := b.EffectiveStorageKey()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, b)
	}
	return out, nil
}
