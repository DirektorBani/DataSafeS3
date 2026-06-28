package metadata

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

type FavoriteRecord struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Type      string    `json:"type"` // bucket, folder
	Bucket    string    `json:"bucket"`
	Prefix    string    `json:"prefix,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookDeliveryRecord struct {
	ID          string    `json:"id"`
	WebhookID   string    `json:"webhook_id"`
	Event       string    `json:"event"`
	URL         string    `json:"url"`
	StatusCode  int       `json:"status_code"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Attempts    int       `json:"attempts"`
	Payload     string    `json:"payload"`
	CreatedAt   time.Time `json:"created_at"`
	LastAttempt time.Time `json:"last_attempt"`
}

type SearchResult struct {
	Type         string `json:"type"` // bucket, object, user
	Name         string `json:"name"`
	Bucket       string `json:"bucket,omitempty"`
	Key          string `json:"key,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Owner        string `json:"owner,omitempty"`
	Username     string `json:"username,omitempty"`
	Email        string `json:"email,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
}

func favoriteKey(userID, id string) []byte {
	return []byte(userID + "\x00" + id)
}

func (s *Store) PutFavorite(rec FavoriteRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("favorites")).Put(favoriteKey(rec.UserID, rec.ID), data)
	})
}

func (s *Store) ListFavorites(userID string) ([]FavoriteRecord, error) {
	var out []FavoriteRecord
	prefix := []byte(userID + "\x00")
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("favorites")).Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var rec FavoriteRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, err
}

func (s *Store) GetFavorite(userID, id string) (FavoriteRecord, error) {
	var rec FavoriteRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("favorites")).Get(favoriteKey(userID, id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) DeleteFavorite(userID, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("favorites"))
		if b.Get(favoriteKey(userID, id)) == nil {
			return ErrNotFound
		}
		return b.Delete(favoriteKey(userID, id))
	})
}

func (s *Store) FindFavorite(userID, favType, bucket, prefix string) (FavoriteRecord, error) {
	items, err := s.ListFavorites(userID)
	if err != nil {
		return FavoriteRecord{}, err
	}
	for _, f := range items {
		if f.Type == favType && f.Bucket == bucket && f.Prefix == prefix {
			return f, nil
		}
	}
	return FavoriteRecord{}, ErrNotFound
}

func (s *Store) PutWebhookDelivery(rec WebhookDeliveryRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("webhook_deliveries")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetWebhookDelivery(id string) (WebhookDeliveryRecord, error) {
	var rec WebhookDeliveryRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("webhook_deliveries")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListWebhookDeliveries(webhookID string, limit int) ([]WebhookDeliveryRecord, error) {
	var out []WebhookDeliveryRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("webhook_deliveries")).ForEach(func(_, v []byte) error {
			var rec WebhookDeliveryRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if webhookID != "" && rec.WebhookID != webhookID {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) SetBucketTags(storageKey string, tags map[string]string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("buckets"))
		data := b.Get([]byte(storageKey))
		if data == nil {
			return ErrNotFound
		}
		var rec BucketRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.Tags = tags
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(storageKey), updated)
	})
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

func (s *Store) UpdateObjectMeta(bucket, key, versionID string, metadata map[string]string, contentType string) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	if metadata != nil {
		rec.Metadata = metadata
	}
	if contentType != "" {
		rec.ContentType = contentType
	}
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}

// ListObjectsPage returns objects after startAfter with pagination.
func (s *Store) ListObjectsPage(bucket, prefix, startAfter string, maxKeys int) ([]ObjectRecord, bool, string, error) {
	all, err := s.ListObjects(bucket, prefix, 0)
	if err != nil {
		return nil, false, "", err
	}
	if maxKeys <= 0 {
		maxKeys = 100
	}
	var filtered []ObjectRecord
	for _, o := range all {
		if startAfter != "" && o.Key <= startAfter {
			continue
		}
		filtered = append(filtered, o)
	}
	truncated := len(filtered) > maxKeys
	if truncated {
		filtered = filtered[:maxKeys]
	}
	nextMarker := ""
	if truncated && len(filtered) > 0 {
		nextMarker = filtered[len(filtered)-1].Key
	}
	return filtered, truncated, nextMarker, nil
}

// Search scans metadata for buckets, objects, and users matching query.
func (s *Store) Search(query string, ownerFilter string, includeUsers bool, offset, limit int) ([]SearchResult, int, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil, 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	type matchKey struct {
		typ, name, bucket, key string
	}
	seen := map[matchKey]bool{}
	var matches []SearchResult

	add := func(r SearchResult) {
		k := matchKey{typ: r.Type, name: r.Name, bucket: r.Bucket, key: r.Key}
		if seen[k] {
			return
		}
		seen[k] = true
		matches = append(matches, r)
	}

	matchText := func(text string) bool {
		t := strings.ToLower(strings.TrimSpace(text))
		if t == "" {
			return false
		}
		if t == q {
			return true
		}
		if strings.HasPrefix(t, q) {
			return true
		}
		return strings.Contains(t, q)
	}

	uuidLike := len(q) >= 8 && !strings.Contains(q, " ")

	buckets, err := s.ListBuckets()
	if err != nil {
		return nil, 0, err
	}
	for _, b := range buckets {
		if ownerFilter != "" && b.Owner != ownerFilter {
			continue
		}
		if matchText(b.Name) || (uuidLike && strings.Contains(strings.ToLower(b.Name), q)) {
			add(SearchResult{
				Type: "bucket", Name: b.Name, Bucket: b.Name, Owner: b.Owner,
			})
		}
		for k, v := range b.Tags {
			if matchText(k) || matchText(v) {
				add(SearchResult{
					Type: "bucket", Name: b.Name, Bucket: b.Name, Owner: b.Owner,
				})
				break
			}
		}
	}

	for _, b := range buckets {
		if ownerFilter != "" && b.Owner != ownerFilter {
			continue
		}
		objs, err := s.ListObjects(b.EffectiveStorageKey(), "", 0)
		if err != nil {
			continue
		}
		for _, o := range objs {
			if matchText(o.Key) || (uuidLike && (matchText(o.VersionID) || strings.Contains(strings.ToLower(o.Key), q))) {
				add(SearchResult{
					Type: "object", Name: o.Key, Bucket: b.Name, Key: o.Key,
					Size: o.Size, LastModified: o.LastModified.UTC().Format(time.RFC3339),
				})
				continue
			}
			for k, v := range o.Tags {
				if matchText(k) || matchText(v) {
					add(SearchResult{
						Type: "object", Name: o.Key, Bucket: b.Name, Key: o.Key,
						Size: o.Size, LastModified: o.LastModified.UTC().Format(time.RFC3339),
					})
					break
				}
			}
			for k, v := range o.Metadata {
				if matchText(k) || matchText(v) {
					add(SearchResult{
						Type: "object", Name: o.Key, Bucket: b.Name, Key: o.Key,
						Size: o.Size, LastModified: o.LastModified.UTC().Format(time.RFC3339),
					})
					break
				}
			}
		}
	}

	if includeUsers {
		users, err := s.ListUsers()
		if err == nil {
			for _, u := range users {
				if matchText(u.Username) || matchText(u.Email) ||
					(uuidLike && strings.Contains(strings.ToLower(u.ID), q)) {
					add(SearchResult{
						Type: "user", Name: u.Username, Username: u.Username, Email: u.Email,
					})
				}
			}
		}
	}

	total := len(matches)
	if offset >= total {
		return []SearchResult{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matches[offset:end], total, nil
}
