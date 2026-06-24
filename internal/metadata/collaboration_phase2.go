package metadata

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

type UserNotificationRecord struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link,omitempty"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type RecentItemRecord struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	Bucket     string    `json:"bucket"`
	Prefix     string    `json:"prefix,omitempty"`
	AccessedAt time.Time `json:"accessed_at"`
}

func notificationKey(userID, id string) []byte {
	return []byte(userID + "\x00" + id)
}

func recentItemKey(userID, id string) []byte {
	return []byte(userID + "\x00" + id)
}

func (s *Store) PutUserNotification(rec UserNotificationRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("user_notifications")).Put(notificationKey(rec.UserID, rec.ID), data)
	})
}

func (s *Store) ListUserNotifications(userID string, limit int) ([]UserNotificationRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []UserNotificationRecord
	prefix := []byte(userID + "\x00")
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("user_notifications")).Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var rec UserNotificationRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) MarkUserNotificationRead(userID, id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("user_notifications"))
		data := b.Get(notificationKey(userID, id))
		if data == nil {
			return ErrNotFound
		}
		var rec UserNotificationRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		now := time.Now().UTC()
		rec.ReadAt = &now
		encoded, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(notificationKey(userID, id), encoded)
	})
}

func (s *Store) MarkAllUserNotificationsRead(userID string) error {
	now := time.Now().UTC()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("user_notifications"))
		prefix := []byte(userID + "\x00")
		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var rec UserNotificationRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.ReadAt != nil {
				continue
			}
			rec.ReadAt = &now
			encoded, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			if err := b.Put(k, encoded); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) CountUnreadNotifications(userID string) (int, error) {
	n := 0
	prefix := []byte(userID + "\x00")
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("user_notifications")).Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var rec UserNotificationRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.ReadAt == nil {
				n++
			}
		}
		return nil
	})
	return n, err
}

func (s *Store) TouchRecentItem(userID, bucket, prefix string) error {
	prefix = strings.TrimSpace(prefix)
	now := time.Now().UTC()
	id := bucket + "\x00" + prefix
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("recent_items"))
		rec := RecentItemRecord{ID: id, UserID: userID, Bucket: bucket, Prefix: prefix, AccessedAt: now}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(recentItemKey(userID, id), data)
	})
}

func (s *Store) ListRecentItems(userID string, limit int) ([]RecentItemRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	var out []RecentItemRecord
	prefix := []byte(userID + "\x00")
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("recent_items")).Cursor()
		for k, v := c.Seek(prefix); k != nil && strings.HasPrefix(string(k), string(prefix)); k, v = c.Next() {
			var rec RecentItemRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].AccessedAt.After(out[j].AccessedAt) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out, err
}
