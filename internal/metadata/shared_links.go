package metadata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	ErrShareExpired      = errors.New("share link expired")
	ErrShareLimitReached = errors.New("share download limit reached")
)

type SharedLinkRecord struct {
	ID            string     `json:"id"`
	Bucket        string     `json:"bucket"`
	Key           string     `json:"key"`
	Token         string     `json:"token,omitempty"`
	TokenHash     string     `json:"token_hash,omitempty"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	MaxDownloads  int        `json:"max_downloads"`
	DownloadCount int        `json:"download_count"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
}

func HashShareToken(token string) string {
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (rec SharedLinkRecord) Active(now time.Time) error {
	if rec.ExpiresAt != nil && now.After(*rec.ExpiresAt) {
		return ErrShareExpired
	}
	if rec.MaxDownloads > 0 && rec.DownloadCount >= rec.MaxDownloads {
		return ErrShareLimitReached
	}
	return nil
}

func (s *Store) PutSharedLink(rec SharedLinkRecord) error {
	hash := HashShareToken(rec.Token)
	if hash == "" {
		return errors.New("share token required")
	}
	storeRec := rec
	storeRec.Token = ""
	storeRec.TokenHash = hash
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(storeRec)
		if err != nil {
			return err
		}
		b := tx.Bucket([]byte("shared_links"))
		if err := b.Put([]byte(rec.ID), data); err != nil {
			return err
		}
		return b.Put([]byte("shared_link_token_hash:"+hash), []byte(rec.ID))
	})
}

func (s *Store) GetSharedLink(id string) (SharedLinkRecord, error) {
	var rec SharedLinkRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("shared_links")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.Token = ""
		return nil
	})
	return rec, err
}

func (s *Store) GetSharedLinkByToken(token string) (SharedLinkRecord, error) {
	var rec SharedLinkRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("shared_links"))
		hash := HashShareToken(token)
		id := b.Get([]byte("shared_link_token_hash:" + hash))
		if id == nil {
			id = b.Get([]byte("shared_link_token:" + token))
		}
		if id == nil {
			return ErrNotFound
		}
		data := b.Get(id)
		if data == nil {
			return ErrNotFound
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.Token = ""
		return nil
	})
	return rec, err
}

func (s *Store) ListSharedLinks(bucket, key string) ([]SharedLinkRecord, error) {
	var out []SharedLinkRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("shared_links")).ForEach(func(k, v []byte) error {
			ks := string(k)
			if strings.HasPrefix(ks, "shared_link_token:") || strings.HasPrefix(ks, "shared_link_token_hash:") {
				return nil
			}
			var rec SharedLinkRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if bucket != "" && rec.Bucket != bucket {
				return nil
			}
			if key != "" && rec.Key != key {
				return nil
			}
			rec.Token = ""
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteSharedLink(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("shared_links"))
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		var rec SharedLinkRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		if rec.TokenHash != "" {
			_ = b.Delete([]byte("shared_link_token_hash:" + rec.TokenHash))
		}
		if rec.Token != "" {
			_ = b.Delete([]byte("shared_link_token:" + rec.Token))
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) IncrementSharedLinkDownload(id string) (SharedLinkRecord, error) {
	var rec SharedLinkRecord
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("shared_links"))
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		now := time.Now().UTC()
		if rec.ExpiresAt != nil && now.After(*rec.ExpiresAt) {
			return ErrShareExpired
		}
		if rec.MaxDownloads > 0 && rec.DownloadCount >= rec.MaxDownloads {
			return ErrShareLimitReached
		}
		rec.DownloadCount++
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), updated)
	})
	return rec, err
}
