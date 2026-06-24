package metadata

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	ErrRetentionLocked = errors.New("object retention period has not expired")
	ErrLegalHold       = errors.New("object is under legal hold")
)

// BucketNotificationConfig holds S3-style event notification rules for a bucket.
type BucketNotificationConfig struct {
	WebhookURL string   `json:"webhook_url,omitempty"`
	Events     []string `json:"events,omitempty"`
}

func BucketNotificationFromTags(tags map[string]string) (BucketNotificationConfig, bool) {
	if tags == nil {
		return BucketNotificationConfig{}, false
	}
	raw, ok := tags[bucketNotificationTagKey]
	if !ok || raw == "" {
		return BucketNotificationConfig{}, false
	}
	var cfg BucketNotificationConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return BucketNotificationConfig{}, false
	}
	return cfg, true
}

func SetBucketNotificationTag(tags map[string]string, cfg BucketNotificationConfig) map[string]string {
	if tags == nil {
		tags = map[string]string{}
	}
	if cfg.WebhookURL == "" && len(cfg.Events) == 0 {
		delete(tags, bucketNotificationTagKey)
		return tags
	}
	data, _ := json.Marshal(cfg)
	tags[bucketNotificationTagKey] = string(data)
	return tags
}

func S3StorageClassDisplay(class string) string {
	switch strings.ToLower(class) {
	case StorageClassHot, "", "standard":
		return StorageClassStandard
	case StorageClassWarm, "standard_ia", "ia":
		return StorageClassIA
	case StorageClassCold, "glacier":
		return StorageClassGlacier
	default:
		if class == StorageClassStandard || class == StorageClassIA || class == StorageClassGlacier {
			return class
		}
		return StorageClassStandard
	}
}

func AdminStorageClassFromS3(class string) string {
	switch strings.ToUpper(class) {
	case StorageClassIA:
		return StorageClassWarm
	case StorageClassGlacier:
		return StorageClassCold
	default:
		return StorageClassHot
	}
}

const (
	StorageClassHot  = "hot"
	StorageClassWarm = "warm"
	StorageClassCold = "cold"

	StorageClassStandard = "STANDARD"
	StorageClassIA       = "STANDARD_IA"
	StorageClassGlacier  = "GLACIER"

	RetentionGovernance = "GOVERNANCE"
	RetentionCompliance = "COMPLIANCE"

	bucketNotificationTagKey = "__datasafe_notification_config"

	DefaultTenantID = "default"

	SyncJobPending   = "pending"
	SyncJobRunning   = "running"
	SyncJobCompleted = "completed"
	SyncJobFailed    = "failed"
)

type LDAPConfig struct {
	Enabled        bool              `json:"enabled"`
	URL            string            `json:"url"`
	BindDN         string            `json:"bind_dn"`
	BindPassword   string            `json:"bind_password,omitempty"`
	BaseDN         string            `json:"base_dn"`
	GroupDN        string            `json:"group_dn,omitempty"`
	UserAttr       string            `json:"user_attr,omitempty"`
	GroupAttr      string            `json:"group_attr,omitempty"`
	GroupRoleMap   map[string]string `json:"group_role_map,omitempty"`
	SyncOnLogin           bool `json:"sync_on_login"`
	SyncIntervalMinutes   int  `json:"sync_interval_minutes,omitempty"`
}

type OIDCConfig struct {
	Enabled         bool   `json:"enabled"`
	Issuer          string `json:"issuer"`
	InternalIssuer  string `json:"internal_issuer,omitempty"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret,omitempty"`
	RedirectURL     string `json:"redirect_url"`
	GroupsClaim     string `json:"groups_claim,omitempty"`
}

type MFASettings struct {
	RequireAdminMFA bool `json:"require_admin_mfa"`
}

type ClusterNode struct {
	ID      string `json:"id"`
	Address string `json:"address"`
	Role    string `json:"role"`
	Status  string `json:"status,omitempty"`
}

type ClusterConfig struct {
	DistributedMode      bool          `json:"distributed_mode"`
	Nodes                []ClusterNode `json:"nodes"`
	ErasureCodingPlanned bool          `json:"erasure_coding_planned"`
	DiskPaths            []string      `json:"disk_paths,omitempty"`
}

type TenantRecord struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type GatewayConnection struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Endpoint   string    `json:"endpoint"`
	Region     string    `json:"region"`
	AccessKey  string    `json:"access_key"`
	SecretKey  string    `json:"secret_key,omitempty"`
	PathStyle  bool      `json:"path_style"`
	TLSVerify  bool      `json:"tls_verify"`
	Status     string    `json:"status,omitempty"`
	LastCheck  time.Time `json:"last_check,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type ReplicationRule struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	SourceBucket    string    `json:"source_bucket"`
	DestConnection  string    `json:"dest_connection_id"`
	DestBucket      string    `json:"dest_bucket"`
	Enabled         bool      `json:"enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

type SyncJob struct {
	ID        string    `json:"id"`
	RuleID    string    `json:"rule_id"`
	Status    string    `json:"status"`
	Objects   int       `json:"objects_synced"`
	Errors    int       `json:"errors"`
	Message   string    `json:"message,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

type FederationCluster struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Endpoint     string    `json:"endpoint"`
	Region       string    `json:"region"`
	Status       string    `json:"status,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *Store) initEnterpriseBuckets(tx *bolt.Tx) error {
	for _, name := range []string{
		"tenants", "tenant_members", "tenant_groups", "tenant_group_buckets", "tenant_group_members",
		"shared_links", "gateway_connections", "replication_rules", "sync_jobs", "federation_clusters",
		"teams", "user_teams",
	} {
		if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
			return err
		}
	}
	return s.initReplicationBuckets(tx)
}

func (s *Store) EnsureDefaultTenant() error {
	_, err := s.GetTenant(DefaultTenantID)
	if err == nil {
		return nil
	}
	return s.PutTenant(TenantRecord{
		ID:        DefaultTenantID,
		Name:      "Default",
		Status:    StatusActive,
		CreatedAt: time.Now().UTC(),
	})
}

func (s *Store) PutTenant(rec TenantRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("tenants")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetTenant(id string) (TenantRecord, error) {
	var rec TenantRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("tenants")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListTenants() ([]TenantRecord, error) {
	var out []TenantRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("tenants")).ForEach(func(_, v []byte) error {
			var rec TenantRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}

func (s *Store) DeleteTenant(id string) error {
	if id == DefaultTenantID {
		return errors.New("cannot delete default tenant")
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("tenants"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) PutGatewayConnection(rec GatewayConnection) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("gateway_connections")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetGatewayConnection(id string) (GatewayConnection, error) {
	var rec GatewayConnection
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("gateway_connections")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListGatewayConnections() ([]GatewayConnection, error) {
	var out []GatewayConnection
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("gateway_connections")).ForEach(func(_, v []byte) error {
			var rec GatewayConnection
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}

func (s *Store) DeleteGatewayConnection(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("gateway_connections"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) PutReplicationRule(rec ReplicationRule) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("replication_rules")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetReplicationRule(id string) (ReplicationRule, error) {
	var rec ReplicationRule
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("replication_rules")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListReplicationRules() ([]ReplicationRule, error) {
	var out []ReplicationRule
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("replication_rules")).ForEach(func(_, v []byte) error {
			var rec ReplicationRule
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) DeleteReplicationRule(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("replication_rules"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) PutSyncJob(rec SyncJob) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("sync_jobs")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetSyncJob(id string) (SyncJob, error) {
	var rec SyncJob
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("sync_jobs")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListSyncJobs(ruleID string, limit int) ([]SyncJob, error) {
	var out []SyncJob
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("sync_jobs")).ForEach(func(_, v []byte) error {
			var rec SyncJob
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if ruleID != "" && rec.RuleID != ruleID {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) PutFederationCluster(rec FederationCluster) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("federation_clusters")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetFederationCluster(id string) (FederationCluster, error) {
	var rec FederationCluster
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("federation_clusters")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListFederationClusters() ([]FederationCluster, error) {
	var out []FederationCluster
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("federation_clusters")).ForEach(func(_, v []byte) error {
			var rec FederationCluster
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, err
}

func (s *Store) DeleteFederationCluster(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("federation_clusters"))
		if b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) ListBucketsByTenant(tenantID string) ([]BucketRecord, error) {
	all, err := s.ListBuckets()
	if err != nil {
		return nil, err
	}
	if tenantID == "" {
		return all, nil
	}
	members, _ := s.ListTenantMembers(tenantID)
	memberOwners := make(map[string]struct{}, len(members))
	for _, m := range members {
		memberOwners[m.UserID] = struct{}{}
	}
	seen := make(map[string]struct{})
	var out []BucketRecord
	for _, b := range all {
		if !bucketBelongsToTenant(b, tenantID, memberOwners) {
			continue
		}
		key := b.EffectiveStorageKey()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, b)
	}
	return out, nil
}

func bucketBelongsToTenant(b BucketRecord, tenantID string, memberOwners map[string]struct{}) bool {
	return BucketBelongsToTenant(b, tenantID, memberOwners)
}

// BucketBelongsToTenant reports whether a bucket is assignable within a tenant namespace.
func BucketBelongsToTenant(b BucketRecord, tenantID string, memberOwners map[string]struct{}) bool {
	if b.EffectiveTenantID() == tenantID {
		return true
	}
	if b.OwnerID != "" {
		if _, ok := memberOwners[b.OwnerID]; ok {
			return true
		}
	}
	return false
}

func (s *Store) SetObjectLegalHold(bucket, key, versionID string, hold bool) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.LegalHold = hold
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}

func (s *Store) SetObjectRetention(bucket, key, versionID string, until time.Time) error {
	rec, err := s.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	rec.RetentionUntil = &until
	if versionID != "" || rec.VersionID != "" {
		return s.PutObjectVersioned(rec)
	}
	return s.PutObject(rec)
}
