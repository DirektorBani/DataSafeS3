package metadata

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

type TeamRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type BucketListFilter struct {
	UserID              string
	Username            string
	TeamIDs             []string
	TenantIDs           []string
	TenantAdminIDs      []string
	GroupBucketKeys     map[string]struct{}
	TenantsWithGroups   map[string]struct{}
	GrantBucketKeys     map[string]struct{}
	Unfiltered          bool
}

func MergeTeamIDs(primary string, extra []string) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(id string) {
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	add(primary)
	for _, id := range extra {
		add(id)
	}
	return out
}

func BucketMatchesFilter(b BucketRecord, f BucketListFilter) bool {
	if f.Unfiltered {
		return true
	}
	if b.OwnerID != "" && b.OwnerID == f.UserID {
		return true
	}
	if b.Owner != "" && b.Owner == f.Username {
		return true
	}
	if b.TeamID != "" {
		for _, tid := range f.TeamIDs {
			if tid == b.TeamID {
				return true
			}
		}
	}
	bucketKey := b.EffectiveStorageKey()
	if f.GrantBucketKeys != nil {
		if _, ok := f.GrantBucketKeys[bucketKey]; ok {
			return true
		}
	}
	if f.GroupBucketKeys != nil {
		if _, ok := f.GroupBucketKeys[bucketKey]; ok {
			return true
		}
	}
	tid := b.EffectiveTenantID()
	if tid == "" {
		return false
	}
	for _, adminTid := range f.TenantAdminIDs {
		if tid == adminTid {
			return true
		}
	}
	_, tenantHasGroups := f.TenantsWithGroups[tid]
	for _, memberTid := range f.TenantIDs {
		if tid != memberTid {
			continue
		}
		if tenantHasGroups {
			return false
		}
		return true
	}
	return false
}

func (s *Store) ListBucketsFiltered(filter BucketListFilter) ([]BucketRecord, error) {
	all, err := s.ListBuckets()
	if err != nil {
		return nil, err
	}
	if filter.Unfiltered {
		return all, nil
	}
	var out []BucketRecord
	for _, b := range all {
		if BucketMatchesFilter(b, filter) {
			out = append(out, b)
		}
	}
	return out, nil
}

func (s *Store) PutTeam(rec TeamRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("teams"))
		if b.Get([]byte(rec.ID)) != nil {
			return ErrUserExists
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetTeam(id string) (TeamRecord, error) {
	var rec TeamRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("teams")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListTeams() ([]TeamRecord, error) {
	var out []TeamRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("teams")).ForEach(func(_, v []byte) error {
			var rec TeamRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteTeam(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("teams")).Get([]byte(id)) == nil {
			return ErrNotFound
		}
		ut := tx.Bucket([]byte("user_teams"))
		if ut != nil {
			var toDelete [][]byte
			_ = ut.ForEach(func(k, _ []byte) error {
				parts := splitUserTeamKey(k)
				if len(parts) == 2 && parts[1] == id {
					toDelete = append(toDelete, append([]byte(nil), k...))
				}
				return nil
			})
			for _, k := range toDelete {
				if err := ut.Delete(k); err != nil {
					return err
				}
			}
		}
		return tx.Bucket([]byte("teams")).Delete([]byte(id))
	})
}

func userTeamKey(userID, teamID string) []byte {
	return []byte(userID + "\x00" + teamID)
}

func splitUserTeamKey(k []byte) []string {
	parts := make([]string, 0, 2)
	start := 0
	for i, ch := range k {
		if ch == 0 {
			parts = append(parts, string(k[start:i]))
			start = i + 1
		}
	}
	if start < len(k) {
		parts = append(parts, string(k[start:]))
	}
	return parts
}

func (s *Store) AddUserTeam(userID, teamID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("user_teams")).Put(userTeamKey(userID, teamID), []byte{1})
	})
}

func (s *Store) RemoveUserTeam(userID, teamID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("user_teams"))
		if b.Get(userTeamKey(userID, teamID)) == nil {
			return ErrNotFound
		}
		return b.Delete(userTeamKey(userID, teamID))
	})
}

func (s *Store) ListUserTeamIDs(userID string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("user_teams"))
		if b == nil {
			return nil
		}
		prefix := []byte(userID + "\x00")
		return b.ForEach(func(k, _ []byte) error {
			if len(k) > len(prefix) && string(k[:len(prefix)]) == string(prefix) {
				parts := splitUserTeamKey(k)
				if len(parts) == 2 {
					out = append(out, parts[1])
				}
			}
			return nil
		})
	})
	return out, err
}
