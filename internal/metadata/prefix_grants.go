package metadata

import (
	"encoding/json"
	"strings"

	bolt "go.etcd.io/bbolt"
)

func prefixGrantKey(bucketKey, userID, prefix string) []byte {
	return []byte(bucketKey + "\x00" + userID + "\x00" + NormalizeSharePrefix(prefix))
}

func (s *Store) PutBucketPrefixAccessGrant(grant BucketPrefixAccessGrant) error {
	grant.Prefix = NormalizeSharePrefix(grant.Prefix)
	if grant.Prefix == "" {
		return ErrInvalidArgument
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(grant)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("bucket_prefix_access_grants")).Put(prefixGrantKey(grant.BucketKey, grant.UserID, grant.Prefix), data)
	})
}

func (s *Store) ListBucketPrefixAccessGrants(bucketKey string) ([]BucketPrefixAccessGrant, error) {
	var out []BucketPrefixAccessGrant
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_prefix_access_grants"))
		if b == nil {
			return nil
		}
		prefix := []byte(bucketKey + "\x00")
		return b.ForEach(func(k, v []byte) error {
			if len(k) < len(prefix) || string(k[:len(prefix)]) != string(prefix) {
				return nil
			}
			var g BucketPrefixAccessGrant
			if err := json.Unmarshal(v, &g); err != nil {
				return err
			}
			out = append(out, g)
			return nil
		})
	})
	return out, err
}

func (s *Store) ListUserPrefixAccessGrants(userID string) ([]BucketPrefixAccessGrant, error) {
	var out []BucketPrefixAccessGrant
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_prefix_access_grants"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			parts := strings.Split(string(k), "\x00")
			if len(parts) < 3 || parts[1] != userID {
				return nil
			}
			var g BucketPrefixAccessGrant
			if err := json.Unmarshal(v, &g); err != nil {
				return err
			}
			out = append(out, g)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteBucketPrefixAccessGrant(bucketKey, userID, prefix string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_prefix_access_grants"))
		key := prefixGrantKey(bucketKey, userID, prefix)
		if b.Get(key) == nil {
			return ErrNotFound
		}
		return b.Delete(key)
	})
}

func (s *Store) DeleteBucketPrefixAccessGrantsForUser(bucketKey, userID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_prefix_access_grants"))
		if b == nil {
			return nil
		}
		prefix := []byte(bucketKey + "\x00" + userID + "\x00")
		var toDelete [][]byte
		_ = b.ForEach(func(k, _ []byte) error {
			if len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix) {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ReplaceBucketPrefixAccessGrants(bucketKey string, grants []BucketPrefixAccessGrant) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_prefix_access_grants"))
		prefix := []byte(bucketKey + "\x00")
		var toDelete [][]byte
		_ = b.ForEach(func(k, _ []byte) error {
			if len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix) {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		for _, g := range grants {
			g.BucketKey = bucketKey
			g.Prefix = NormalizeSharePrefix(g.Prefix)
			if g.Prefix == "" {
				continue
			}
			data, err := json.Marshal(g)
			if err != nil {
				return err
			}
			if err := b.Put(prefixGrantKey(bucketKey, g.UserID, g.Prefix), data); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) CountBucketPrefixAccessGrants(bucketKey string) (int, error) {
	n := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_prefix_access_grants"))
		if b == nil {
			return nil
		}
		prefix := []byte(bucketKey + "\x00")
		return b.ForEach(func(k, _ []byte) error {
			if len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix) {
				n++
			}
			return nil
		})
	})
	return n, err
}
