package metadata

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	TrashBucketName         = ".datasafe-trash"
	EventObjectCreated      = "ObjectCreated"
	EventObjectDeleted      = "ObjectDeleted"
	EventBucketCreated      = "BucketCreated"
	EventBucketDeleted      = "BucketDeleted"
	EventUserCreated        = "UserCreated"
	EventMultipartCompleted = "MultipartCompleted"
)

type LogSinkEndpoint struct {
	Enabled  bool              `json:"enabled"`
	Address  string            `json:"address,omitempty"`
	Username string            `json:"username,omitempty"`
	Password string            `json:"password,omitempty"`
	Token    string            `json:"token,omitempty"`
	TLS      bool              `json:"tls,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Index    string            `json:"index,omitempty"`
}

type LoggingConfig struct {
	Syslog        LogSinkEndpoint `json:"syslog,omitempty"`
	Loki          LogSinkEndpoint `json:"loki,omitempty"`
	Elasticsearch LogSinkEndpoint `json:"elasticsearch,omitempty"`
	Webhook       LogSinkEndpoint `json:"webhook,omitempty"`
}

// ExternalS3Config stores credentials for an external S3-compatible endpoint
// (backups, gateway replication). Primary object storage remains local FSBackend.
type ExternalS3Config struct {
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	UseSSL          bool   `json:"use_ssl"`
}

type SystemConfig struct {
	InitialSetupCompleted    bool             `json:"initial_setup_completed"`
	AdminFirstLoginCompleted bool             `json:"admin_first_login_completed"`
	AdminPasswordChanged     bool             `json:"admin_password_changed"`
	SoftDeleteEnabled        bool             `json:"soft_delete_enabled"`
	TrashRetentionDays       int              `json:"trash_retention_days"` // 1–3650 days
	ExternalS3               ExternalS3Config `json:"external_s3,omitempty"`
	LDAP                     LDAPConfig       `json:"ldap,omitempty"`
	OIDC                     OIDCConfig       `json:"oidc,omitempty"`
	MFA                      MFASettings      `json:"mfa,omitempty"`
	Cluster                  ClusterConfig    `json:"cluster,omitempty"`
	Logging                  LoggingConfig    `json:"logging,omitempty"`
}

type TrashRecord struct {
	ID             string    `json:"id"`
	OriginalBucket string    `json:"original_bucket"`
	OriginalKey    string    `json:"original_key"`
	TrashKey       string    `json:"trash_key"`
	Size           int64     `json:"size"`
	VersionID      string    `json:"version_id,omitempty"`
	DeletedBy      string    `json:"deleted_by,omitempty"`
	DeletedAt      time.Time `json:"deleted_at"`
}

type ConsoleTokenRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	TokenHash string    `json:"token_hash"`
	UserID    string    `json:"user_id"`
	Username  string    `json:"username"`
	Scopes    []string  `json:"scopes"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type WebhookRecord struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	Events    []string          `json:"events"`
	Headers   map[string]string `json:"headers,omitempty"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
}

func (s *Store) GetSystemConfig() (SystemConfig, error) {
	var cfg SystemConfig
	cfg.TrashRetentionDays = 30
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("config")).Get([]byte("system"))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &cfg)
	})
	if err != nil {
		return cfg, err
	}
	return DecryptSystemConfigPaths(s.fieldenc, cfg)
}

func (s *Store) PutSystemConfig(cfg SystemConfig) error {
	var err error
	cfg, err = EncryptSystemConfigPaths(s.fieldenc, cfg)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("config")).Put([]byte("system"), data)
	})
}

func (s *Store) PutTrash(rec TrashRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("trash")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetTrash(id string) (TrashRecord, error) {
	var rec TrashRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("trash")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) DeleteTrash(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("trash"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) ListTrash(bucketFilter string) ([]TrashRecord, error) {
	var out []TrashRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("trash")).ForEach(func(_, v []byte) error {
			var rec TrashRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if bucketFilter != "" && rec.OriginalBucket != bucketFilter {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) PutConsoleToken(rec ConsoleTokenRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("api_tokens")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetConsoleToken(id string) (ConsoleTokenRecord, error) {
	var rec ConsoleTokenRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("api_tokens")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) FindConsoleTokenByHash(hash string) (ConsoleTokenRecord, error) {
	var found ConsoleTokenRecord
	var ok bool
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("api_tokens")).ForEach(func(_, v []byte) error {
			var rec ConsoleTokenRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.TokenHash == hash {
				found = rec
				ok = true
			}
			return nil
		})
	})
	if err != nil {
		return ConsoleTokenRecord{}, err
	}
	if !ok {
		return ConsoleTokenRecord{}, ErrNotFound
	}
	return found, nil
}

func (s *Store) ListConsoleTokens(userID string) ([]ConsoleTokenRecord, error) {
	var out []ConsoleTokenRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("api_tokens")).ForEach(func(_, v []byte) error {
			var rec ConsoleTokenRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if userID != "" && rec.UserID != userID {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteConsoleToken(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("api_tokens"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) PutWebhook(rec WebhookRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("webhooks")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetWebhook(id string) (WebhookRecord, error) {
	var rec WebhookRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("webhooks")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListWebhooks() ([]WebhookRecord, error) {
	var out []WebhookRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("webhooks")).ForEach(func(_, v []byte) error {
			var rec WebhookRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteWebhook(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("webhooks"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}
