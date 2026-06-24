package metadata

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	GroupAccessRead      = "read"
	GroupAccessReadWrite = "read_write"
)

var (
	ErrTenantGroupExists = errors.New("tenant group already exists")
	ErrTenantGroupName   = errors.New("invalid tenant group name")
)

type TenantGroupRecord struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Name         string    `json:"name"`
	ExternalName string    `json:"external_name,omitempty"`
	Description  string    `json:"description,omitempty"`
	AccessLevel  string    `json:"access_level"`
	CreatedAt    time.Time `json:"created_at"`
}

type TenantGroupBucket struct {
	GroupID   string `json:"group_id"`
	BucketKey string `json:"bucket_key"`
}

type TenantGroupMember struct {
	GroupID string `json:"group_id"`
	UserID  string `json:"user_id"`
}

// UserGroupBucketAccess describes group-based access to a bucket for RBAC.
type UserGroupBucketAccess struct {
	BucketKey   string
	CanRead     bool
	CanWrite    bool
	AccessLevel string
}

func normalizeGroupAccessLevel(level string) string {
	switch level {
	case GroupAccessReadWrite:
		return GroupAccessReadWrite
	default:
		return GroupAccessRead
	}
}

func groupBucketKey(groupID, bucketKey string) []byte {
	return []byte(groupID + "\x00" + bucketKey)
}

func groupMemberKey(groupID, userID string) []byte {
	return []byte(groupID + "\x00" + userID)
}

func splitGroupPairKey(k []byte) (string, string) {
	s := string(k)
	for i, ch := range s {
		if ch == 0 {
			return s[:i], s[i+1:]
		}
	}
	return s, ""
}

func (s *Store) PutTenantGroup(rec TenantGroupRecord) error {
	if rec.Name == "" {
		return ErrTenantGroupName
	}
	rec.AccessLevel = normalizeGroupAccessLevel(rec.AccessLevel)
	return s.db.Update(func(tx *bolt.Tx) error {
		groups := tx.Bucket([]byte("tenant_groups"))
		err := groups.ForEach(func(_, v []byte) error {
			var existing TenantGroupRecord
			if err := json.Unmarshal(v, &existing); err != nil {
				return err
			}
			if existing.TenantID == rec.TenantID && existing.Name == rec.Name && existing.ID != rec.ID {
				return ErrTenantGroupExists
			}
			return nil
		})
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return groups.Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetTenantGroup(id string) (TenantGroupRecord, error) {
	var rec TenantGroupRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("tenant_groups")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListTenantGroups(tenantID string) ([]TenantGroupRecord, error) {
	var out []TenantGroupRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_groups")).ForEach(func(_, v []byte) error {
			var rec TenantGroupRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.TenantID == tenantID {
				out = append(out, rec)
			}
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}

func (s *Store) DeleteTenantGroup(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		groups := tx.Bucket([]byte("tenant_groups"))
		if groups.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		buckets := tx.Bucket([]byte("tenant_group_buckets"))
		var bucketKeys [][]byte
		_ = buckets.ForEach(func(k, _ []byte) error {
			gid, _ := splitGroupPairKey(k)
			if gid == id {
				bucketKeys = append(bucketKeys, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range bucketKeys {
			if err := buckets.Delete(k); err != nil {
				return err
			}
		}
		members := tx.Bucket([]byte("tenant_group_members"))
		var memberKeys [][]byte
		_ = members.ForEach(func(k, _ []byte) error {
			gid, _ := splitGroupPairKey(k)
			if gid == id {
				memberKeys = append(memberKeys, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range memberKeys {
			if err := members.Delete(k); err != nil {
				return err
			}
		}
		return groups.Delete([]byte(id))
	})
}

func (s *Store) CountTenantGroups(tenantID string) (int, error) {
	n := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_groups")).ForEach(func(_, v []byte) error {
			var rec TenantGroupRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.TenantID == tenantID {
				n++
			}
			return nil
		})
	})
	return n, err
}

func (s *Store) ReplaceTenantGroupBuckets(groupID string, bucketKeys []string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if tx.Bucket([]byte("tenant_groups")).Get([]byte(groupID)) == nil {
			return ErrNotFound
		}
		b := tx.Bucket([]byte("tenant_group_buckets"))
		var toDelete [][]byte
		_ = b.ForEach(func(k, _ []byte) error {
			gid, _ := splitGroupPairKey(k)
			if gid == groupID {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		for _, bk := range bucketKeys {
			if bk == "" {
				continue
			}
			if err := b.Put(groupBucketKey(groupID, bk), []byte{1}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListTenantGroupBuckets(groupID string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_group_buckets")).ForEach(func(k, _ []byte) error {
			gid, bk := splitGroupPairKey(k)
			if gid == groupID {
				out = append(out, bk)
			}
			return nil
		})
	})
	sort.Strings(out)
	return out, err
}

func (s *Store) ListTenantGroupBucketKeys(tenantID string) ([]string, error) {
	groups, err := s.ListTenantGroups(tenantID)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, g := range groups {
		keys, err := s.ListTenantGroupBuckets(g.ID)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if _, ok := seen[k]; !ok {
				seen[k] = struct{}{}
				out = append(out, k)
			}
		}
	}
	sort.Strings(out)
	return out, err
}

func (s *Store) ReplaceUserTenantGroups(tenantID, userID string, groupIDs []string) error {
	groups, err := s.ListTenantGroups(tenantID)
	if err != nil {
		return err
	}
	valid := map[string]struct{}{}
	for _, g := range groups {
		valid[g.ID] = struct{}{}
	}
	for _, gid := range groupIDs {
		if _, ok := valid[gid]; !ok {
			return ErrNotFound
		}
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("tenant_group_members"))
		var toDelete [][]byte
		_ = b.ForEach(func(k, _ []byte) error {
			gid, uid := splitGroupPairKey(k)
			if uid != userID {
				return nil
			}
			if tenantGroupInBucket(tx, gid, tenantID) {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range toDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		for _, gid := range groupIDs {
			if err := b.Put(groupMemberKey(gid, userID), []byte{1}); err != nil {
				return err
			}
		}
		return nil
	})
}

func tenantGroupInBucket(tx *bolt.Tx, groupID, tenantID string) bool {
	data := tx.Bucket([]byte("tenant_groups")).Get([]byte(groupID))
	if data == nil {
		return false
	}
	var rec TenantGroupRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return false
	}
	return rec.TenantID == tenantID
}

func (s *Store) ListUserTenantGroupIDs(tenantID, userID string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_group_members")).ForEach(func(k, _ []byte) error {
			gid, uid := splitGroupPairKey(k)
			if uid != userID {
				return nil
			}
			if tenantGroupInBucket(tx, gid, tenantID) {
				out = append(out, gid)
			}
			return nil
		})
	})
	sort.Strings(out)
	return out, err
}

func (s *Store) ListTenantGroupMembers(groupID string) ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_group_members")).ForEach(func(k, _ []byte) error {
			gid, uid := splitGroupPairKey(k)
			if gid == groupID {
				out = append(out, uid)
			}
			return nil
		})
	})
	sort.Strings(out)
	return out, err
}

func (s *Store) ListUserGroupBucketAccess(userID string) ([]UserGroupBucketAccess, error) {
	byBucket := map[string]UserGroupBucketAccess{}
	err := s.db.View(func(tx *bolt.Tx) error {
		groups := tx.Bucket([]byte("tenant_groups"))
		return tx.Bucket([]byte("tenant_group_members")).ForEach(func(k, _ []byte) error {
			gid, uid := splitGroupPairKey(k)
			if uid != userID {
				return nil
			}
			gdata := groups.Get([]byte(gid))
			if gdata == nil {
				return nil
			}
			var g TenantGroupRecord
			if err := json.Unmarshal(gdata, &g); err != nil {
				return err
			}
			canWrite := g.AccessLevel == GroupAccessReadWrite
			return tx.Bucket([]byte("tenant_group_buckets")).ForEach(func(bk, _ []byte) error {
				bgid, bucketKey := splitGroupPairKey(bk)
				if bgid != gid {
					return nil
				}
				cur, ok := byBucket[bucketKey]
				if !ok {
					cur = UserGroupBucketAccess{
						BucketKey:   bucketKey,
						CanRead:     true,
						CanWrite:    canWrite,
						AccessLevel: g.AccessLevel,
					}
				} else {
					cur.CanRead = true
					if canWrite {
						cur.CanWrite = true
						cur.AccessLevel = GroupAccessReadWrite
					}
				}
				byBucket[bucketKey] = cur
				return nil
			})
		})
	})
	if err != nil {
		return nil, err
	}
	out := make([]UserGroupBucketAccess, 0, len(byBucket))
	for _, v := range byBucket {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BucketKey < out[j].BucketKey })
	return out, nil
}

func (s *Store) RemoveUserFromTenantGroups(tenantID, userID string) error {
	return s.ReplaceUserTenantGroups(tenantID, userID, nil)
}
