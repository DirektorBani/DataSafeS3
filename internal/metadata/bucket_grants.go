package metadata

import (
	"encoding/json"

	bolt "go.etcd.io/bbolt"
)

func grantKey(bucketKey, userID string) []byte {
	return []byte(bucketKey + "\x00" + userID)
}

func splitGrantKey(k []byte) (string, string) {
	s := string(k)
	i := 0
	for i < len(s) && s[i] != '\x00' {
		i++
	}
	if i >= len(s) {
		return s, ""
	}
	return s[:i], s[i+1:]
}

func (s *Store) PutBucketAccessGrant(grant BucketAccessGrant) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(grant)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("bucket_access_grants")).Put(grantKey(grant.BucketKey, grant.UserID), data)
	})
}

func (s *Store) ListBucketAccessGrants(bucketKey string) ([]BucketAccessGrant, error) {
	var out []BucketAccessGrant
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_access_grants"))
		if b == nil {
			return nil
		}
		prefix := []byte(bucketKey + "\x00")
		return b.ForEach(func(k, v []byte) error {
			if len(k) < len(prefix) || string(k[:len(prefix)]) != string(prefix) {
				return nil
			}
			var g BucketAccessGrant
			if err := json.Unmarshal(v, &g); err != nil {
				return err
			}
			out = append(out, g)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteBucketAccessGrant(bucketKey, userID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_access_grants"))
		if b.Get(grantKey(bucketKey, userID)) == nil {
			return ErrNotFound
		}
		return b.Delete(grantKey(bucketKey, userID))
	})
}

func (s *Store) ReplaceBucketAccessGrants(bucketKey string, grants []BucketAccessGrant) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_access_grants"))
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
			data, err := json.Marshal(g)
			if err != nil {
				return err
			}
			if err := b.Put(grantKey(bucketKey, g.UserID), data); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) CountBucketAccessGrants(bucketKey string) (int, error) {
	n := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("bucket_access_grants"))
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
