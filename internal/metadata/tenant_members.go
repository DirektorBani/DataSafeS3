package metadata

import (
	"encoding/json"
	"errors"
	"sort"

	bolt "go.etcd.io/bbolt"
)

const (
	TenantRoleAdmin  = "tenant_admin"
	TenantRoleMember = "member"
	TenantRoleViewer = "viewer"
)

var ErrTenantMemberExists = errors.New("tenant member already exists")

type TenantMemberRecord struct {
	TenantID string `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Role     string `json:"role"`
}

func tenantMemberKey(tenantID, userID string) []byte {
	return []byte(tenantID + ":" + userID)
}

func (s *Store) PutTenantMember(rec TenantMemberRecord) error {
	if rec.Role == "" {
		rec.Role = TenantRoleMember
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("tenant_members"))
		key := tenantMemberKey(rec.TenantID, rec.UserID)
		if b.Get(key) != nil {
			return ErrTenantMemberExists
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(key, data)
	})
}

func (s *Store) GetTenantMember(tenantID, userID string) (TenantMemberRecord, error) {
	var rec TenantMemberRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("tenant_members")).Get(tenantMemberKey(tenantID, userID))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListTenantMembers(tenantID string) ([]TenantMemberRecord, error) {
	var out []TenantMemberRecord
	prefix := tenantID + ":"
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_members")).ForEach(func(k, v []byte) error {
			if len(k) <= len(prefix) || string(k[:len(prefix)]) != prefix {
				return nil
			}
			var rec TenantMemberRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, err
}

func (s *Store) ListUserTenants(userID string) ([]TenantMemberRecord, error) {
	var out []TenantMemberRecord
	suffix := ":" + userID
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenant_members")).ForEach(func(k, v []byte) error {
			ks := string(k)
			if len(ks) <= len(suffix) || ks[len(ks)-len(suffix):] != suffix {
				return nil
			}
			var rec TenantMemberRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].TenantID < out[j].TenantID })
	return out, err
}

func (s *Store) UpdateTenantMemberRole(tenantID, userID, role string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("tenant_members"))
		key := tenantMemberKey(tenantID, userID)
		data := b.Get(key)
		if data == nil {
			return ErrNotFound
		}
		var rec TenantMemberRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.Role = role
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(key, updated)
	})
}

func (s *Store) DeleteTenantMember(tenantID, userID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("tenant_members"))
		key := tenantMemberKey(tenantID, userID)
		if b.Get(key) == nil {
			return ErrNotFound
		}
		return b.Delete(key)
	})
}
