package metadata

import (
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	ErrUserExists   = errors.New("user already exists")
	ErrUserNotFound = errors.New("user not found")
	ErrLastAdmin    = errors.New("cannot delete last administrator")
)

const (
	RoleAdministrator = "administrator"
	RoleOperator      = "operator"
	RoleUser          = "user"

	StatusActive    = "active"
	StatusSuspended = "suspended"
)

type UserRecord struct {
	ID                  string     `json:"id"`
	Username            string     `json:"username"`
	Email               string     `json:"email"`
	PasswordHash        string     `json:"password_hash"`
	Role                string     `json:"role"`
	Status              string     `json:"status"`
	TenantID            string     `json:"tenant_id,omitempty"`
	TeamID              string     `json:"team_id,omitempty"`
	MFAEnabled          bool       `json:"mfa_enabled,omitempty"`
	TOTPSecret          string     `json:"totp_secret,omitempty"`
	RecoveryCodes       []string   `json:"recovery_codes,omitempty"`
	AuthSource          string     `json:"auth_source,omitempty"`
	MaxSizeBytes        int64      `json:"max_size_bytes,omitempty"`
	MaxObjects          int64      `json:"max_objects,omitempty"`
	LastLogin           *time.Time `json:"last_login,omitempty"`
	Locale              string     `json:"locale,omitempty"`
	WebAuthnCredentials string     `json:"webauthn_credentials,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

func (s *Store) PutUser(rec UserRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b.Get([]byte(rec.ID)) != nil {
			return ErrUserExists
		}
		idx := tx.Bucket([]byte("user_index"))
		if idx.Get([]byte(rec.Username)) != nil {
			return ErrUserExists
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		if err := b.Put([]byte(rec.ID), data); err != nil {
			return err
		}
		return idx.Put([]byte(rec.Username), []byte(rec.ID))
	})
}

func (s *Store) GetUser(id string) (UserRecord, error) {
	var rec UserRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("users")).Get([]byte(id))
		if data == nil {
			return ErrUserNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) GetUserByUsername(username string) (UserRecord, error) {
	var rec UserRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		id := tx.Bucket([]byte("user_index")).Get([]byte(username))
		if id == nil {
			return ErrUserNotFound
		}
		data := tx.Bucket([]byte("users")).Get(id)
		if data == nil {
			return ErrUserNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) UpdateUser(rec UserRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		if b.Get([]byte(rec.ID)) == nil {
			return ErrUserNotFound
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(rec.ID), data)
	})
}

func (s *Store) DeleteUser(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		data := b.Get([]byte(id))
		if data == nil {
			return ErrUserNotFound
		}
		var rec UserRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		if rec.Role == RoleAdministrator {
			admins := 0
			_ = b.ForEach(func(_, v []byte) error {
				var u UserRecord
				if json.Unmarshal(v, &u) == nil && u.Role == RoleAdministrator {
					admins++
				}
				return nil
			})
			if admins <= 1 {
				return ErrLastAdmin
			}
		}
		if err := tx.Bucket([]byte("user_index")).Delete([]byte(rec.Username)); err != nil {
			return err
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) ListUsers() ([]UserRecord, error) {
	var out []UserRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("users")).ForEach(func(_, v []byte) error {
			var rec UserRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("users")).ForEach(func(_, _ []byte) error {
			n++
			return nil
		})
	})
	return n, err
}
