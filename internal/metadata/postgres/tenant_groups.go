package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Store) PutTenantGroup(rec metadata.TenantGroupRecord) error {
	if rec.AccessLevel == "" {
		rec.AccessLevel = metadata.GroupAccessRead
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO tenant_groups (id, tenant_id, name, external_name, description, access_level, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (id) DO UPDATE SET
			name=EXCLUDED.name, external_name=EXCLUDED.external_name,
			description=EXCLUDED.description, access_level=EXCLUDED.access_level`,
		rec.ID, rec.TenantID, rec.Name, rec.ExternalName, rec.Description, rec.AccessLevel, rec.CreatedAt)
	return err
}

func (s *Store) GetTenantGroup(id string) (metadata.TenantGroupRecord, error) {
	var rec metadata.TenantGroupRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, tenant_id, name, external_name, description, access_level, created_at
		FROM tenant_groups WHERE id=$1`, id).
		Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.ExternalName, &rec.Description, &rec.AccessLevel, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) ListTenantGroups(tenantID string) ([]metadata.TenantGroupRecord, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, tenant_id, name, external_name, description, access_level, created_at
		FROM tenant_groups WHERE tenant_id=$1 ORDER BY name`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.TenantGroupRecord
	for rows.Next() {
		var rec metadata.TenantGroupRecord
		if err := rows.Scan(&rec.ID, &rec.TenantID, &rec.Name, &rec.ExternalName, &rec.Description, &rec.AccessLevel, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTenantGroup(id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM tenant_groups WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) CountTenantGroups(tenantID string) (int, error) {
	var n int
	err := s.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM tenant_groups WHERE tenant_id=$1`, tenantID).Scan(&n)
	return n, err
}

func (s *Store) ReplaceTenantGroupBuckets(groupID string, bucketKeys []string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM tenant_group_buckets WHERE group_id=$1`, groupID); err != nil {
		return err
	}
	for _, bk := range bucketKeys {
		if bk == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tenant_group_buckets (group_id, bucket_key) VALUES ($1,$2)`, groupID, bk); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListTenantGroupBuckets(groupID string) ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT bucket_key FROM tenant_group_buckets WHERE group_id=$1 ORDER BY bucket_key`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) ListTenantGroupBucketKeys(tenantID string) ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT DISTINCT tgb.bucket_key
		FROM tenant_group_buckets tgb
		JOIN tenant_groups tg ON tg.id = tgb.group_id
		WHERE tg.tenant_id = $1
		ORDER BY tgb.bucket_key`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceUserTenantGroups(tenantID, userID string, groupIDs []string) error {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		DELETE FROM tenant_group_members tgm
		USING tenant_groups tg
		WHERE tgm.group_id = tg.id AND tg.tenant_id = $1 AND tgm.user_id = $2`,
		tenantID, userID); err != nil {
		return err
	}
	for _, gid := range groupIDs {
		var ok bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM tenant_groups WHERE id=$1 AND tenant_id=$2)`, gid, tenantID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return metadata.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tenant_group_members (group_id, user_id) VALUES ($1,$2)`, gid, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListUserTenantGroupIDs(tenantID, userID string) ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT tgm.group_id
		FROM tenant_group_members tgm
		JOIN tenant_groups tg ON tg.id = tgm.group_id
		WHERE tg.tenant_id = $1 AND tgm.user_id = $2
		ORDER BY tgm.group_id`, tenantID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ListTenantGroupMembers(groupID string) ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT user_id FROM tenant_group_members WHERE group_id=$1 ORDER BY user_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) ListUserGroupBucketAccess(userID string) ([]metadata.UserGroupBucketAccess, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT tgb.bucket_key, tg.access_level
		FROM tenant_group_members tgm
		JOIN tenant_groups tg ON tg.id = tgm.group_id
		JOIN tenant_group_buckets tgb ON tgb.group_id = tg.id
		WHERE tgm.user_id = $1
		ORDER BY tgb.bucket_key`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byBucket := map[string]metadata.UserGroupBucketAccess{}
	for rows.Next() {
		var bucketKey, accessLevel string
		if err := rows.Scan(&bucketKey, &accessLevel); err != nil {
			return nil, err
		}
		canWrite := accessLevel == metadata.GroupAccessReadWrite
		cur, ok := byBucket[bucketKey]
		if !ok {
			cur = metadata.UserGroupBucketAccess{
				BucketKey: bucketKey, CanRead: true, CanWrite: canWrite, AccessLevel: accessLevel,
			}
		} else {
			cur.CanRead = true
			if canWrite {
				cur.CanWrite = true
				cur.AccessLevel = metadata.GroupAccessReadWrite
			}
		}
		byBucket[bucketKey] = cur
	}
	out := make([]metadata.UserGroupBucketAccess, 0, len(byBucket))
	for _, v := range byBucket {
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) RemoveUserFromTenantGroups(tenantID, userID string) error {
	return s.ReplaceUserTenantGroups(tenantID, userID, nil)
}
