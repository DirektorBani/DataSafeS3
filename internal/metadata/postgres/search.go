package postgres

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func (s *Store) PutFavorite(rec metadata.FavoriteRecord) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO favorites (id, user_id, type, bucket, prefix, created_at) VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (user_id, id) DO UPDATE SET type=$3, bucket=$4, prefix=$5`,
		rec.ID, rec.UserID, rec.Type, rec.Bucket, rec.Prefix, rec.CreatedAt)
	return err
}

func (s *Store) ListFavorites(userID string) ([]metadata.FavoriteRecord, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT id, user_id, type, bucket, COALESCE(prefix,''), created_at
		FROM favorites WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.FavoriteRecord
	for rows.Next() {
		var rec metadata.FavoriteRecord
		if err := rows.Scan(&rec.ID, &rec.UserID, &rec.Type, &rec.Bucket, &rec.Prefix, &rec.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) GetFavorite(userID, id string) (metadata.FavoriteRecord, error) {
	var rec metadata.FavoriteRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, user_id, type, bucket, COALESCE(prefix,''), created_at
		FROM favorites WHERE user_id=$1 AND id=$2`, userID, id).Scan(
		&rec.ID, &rec.UserID, &rec.Type, &rec.Bucket, &rec.Prefix, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) DeleteFavorite(userID, id string) error {
	tag, err := s.pool.Exec(context.Background(), `DELETE FROM favorites WHERE user_id=$1 AND id=$2`, userID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) FindFavorite(userID, favType, bucket, prefix string) (metadata.FavoriteRecord, error) {
	items, err := s.ListFavorites(userID)
	if err != nil {
		return metadata.FavoriteRecord{}, err
	}
	for _, f := range items {
		if f.Type == favType && f.Bucket == bucket && f.Prefix == prefix {
			return f, nil
		}
	}
	return metadata.FavoriteRecord{}, metadata.ErrNotFound
}

// Search uses pg_trgm for fuzzy matching when postgres backend is active.
func (s *Store) Search(query string, ownerFilter string, includeUsers bool, offset, limit int) ([]metadata.SearchResult, int, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	ctx := context.Background()
	var results []metadata.SearchResult
	pattern := "%" + q + "%"

	// Buckets
	bucketRows, err := s.readQueryPool().Query(ctx, `
		SELECT name, owner FROM buckets
		WHERE (name ILIKE $1 OR name % $2)
			AND ($3 = '' OR owner = $3)
		ORDER BY similarity(name, $2) DESC NULLS LAST, name LIMIT 50`, pattern, q, ownerFilter)
	if err == nil {
		for bucketRows.Next() {
			var name, owner string
			if err := bucketRows.Scan(&name, &owner); err != nil {
				break
			}
			results = append(results, metadata.SearchResult{Type: "bucket", Name: name, Owner: owner})
		}
		bucketRows.Close()
	}

	// Objects
	objRows, err := s.readQueryPool().Query(ctx, `
		SELECT o.bucket, o.key, o.size, o.last_modified, b.owner
		FROM objects o JOIN buckets b ON b.storage_key=o.bucket
		WHERE o.is_latest=TRUE AND o.is_delete_marker=FALSE
			AND (o.key ILIKE $1 OR o.key % $2 OR o.metadata::text ILIKE $1)
			AND ($3 = '' OR b.owner = $3)
		ORDER BY similarity(o.key, $2) DESC NULLS LAST LIMIT 100`, pattern, q, ownerFilter)
	if err == nil {
		for objRows.Next() {
			var bucket, key, owner string
			var size int64
			var mod time.Time
			if err := objRows.Scan(&bucket, &key, &size, &mod, &owner); err != nil {
				break
			}
			results = append(results, metadata.SearchResult{
				Type: "object", Name: key, Bucket: bucket, Key: key, Size: size, Owner: owner,
				LastModified: mod.UTC().Format(time.RFC3339),
			})
		}
		objRows.Close()
	}

	if includeUsers {
		userRows, err := s.pool.Query(ctx, `
			SELECT username, email FROM users
			WHERE username ILIKE $1 OR username % $2 OR email ILIKE $1
			ORDER BY similarity(username, $2) DESC NULLS LAST LIMIT 20`, pattern, q)
		if err == nil {
			for userRows.Next() {
				var user, email string
				if err := userRows.Scan(&user, &email); err != nil {
					break
				}
				results = append(results, metadata.SearchResult{Type: "user", Name: user, Username: user, Email: email})
			}
			userRows.Close()
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})
	total := len(results)
	if offset >= total {
		return []metadata.SearchResult{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return results[offset:end], total, nil
}
