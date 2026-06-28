package metadata

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	ActionLogin                 = "login"
	ActionLogout                = "logout"
	ActionBucketCreated         = "bucket_created"
	ActionBucketDeleted         = "bucket_deleted"
	ActionBucketUpdated         = "bucket_updated"
	ActionObjectUploaded        = "object_uploaded"
	ActionObjectDownloaded      = "object_downloaded"
	ActionObjectDeleted         = "object_deleted"
	ActionObjectScheduledDelete = "object_scheduled_delete"
	ActionAccessKeyCreated      = "access_key_created"
	ActionAccessKeyDeleted      = "access_key_deleted"
	ActionUserCreated           = "user_created"
	ActionUserDeleted           = "user_deleted"
	ActionSettingsChanged       = "settings_changed"
	ActionPolicyChanged         = "policy_changed"
	ActionTrashRestored         = "trash_restored"
	ActionTrashPurged           = "trash_purged"
	ActionGatewayReplicated     = "gateway_replicated"
	ActionGatewayReplFailed     = "gateway_replication_failed"
	ActionShareCreated          = "share.created"
	ActionShareDownloaded       = "share.downloaded"
	ActionShareLimitReached     = "share.limit_reached"
	ActionShareExpired          = "share.expired"
)

type ActivityRecord struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	User         string    `json:"user"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceName string    `json:"resource_name"`
	IPAddress    string    `json:"ip_address"`
}

type ActivityFilter struct {
	Period    string // 24h, 7d, 30d, all
	User      string
	Action    string
	Bucket    string
	IP        string
	Search    string
	LimitUser string // RBAC: restrict to this username
	Offset    int
	Limit     int
}

type ActivityListResult struct {
	Events []ActivityRecord `json:"events"`
	Total  int              `json:"total"`
}

func activityKey(ts time.Time, id string) []byte {
	return []byte(fmt.Sprintf("%019d\x00%s", ts.UnixNano(), id))
}

func (s *Store) AppendActivity(rec ActivityRecord) error {
	if rec.Timestamp.IsZero() {
		rec.Timestamp = time.Now().UTC()
	}
	if rec.ID == "" {
		rec.ID = fmt.Sprintf("%d", rec.Timestamp.UnixNano())
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("activity")).Put(activityKey(rec.Timestamp, rec.ID), data)
	})
}

func (s *Store) ListActivity(f ActivityFilter) (ActivityListResult, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	var all []ActivityRecord
	var since time.Time
	switch f.Period {
	case "24h":
		since = time.Now().UTC().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().UTC().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().UTC().Add(-30 * 24 * time.Hour)
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("activity"))
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var rec ActivityRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if !since.IsZero() && rec.Timestamp.Before(since) {
				break
			}
			if f.LimitUser != "" && rec.User != f.LimitUser {
				continue
			}
			if f.User != "" && rec.User != f.User {
				continue
			}
			if f.Action != "" && rec.Action != f.Action {
				continue
			}
			if f.Bucket != "" && rec.ResourceType == "bucket" && rec.ResourceName != f.Bucket {
				continue
			}
			if f.Bucket != "" && rec.ResourceType == "object" && !strings.HasPrefix(rec.ResourceName, f.Bucket+"/") {
				continue
			}
			if f.IP != "" && rec.IPAddress != f.IP {
				continue
			}
			if f.Search != "" {
				q := strings.ToLower(f.Search)
				hay := strings.ToLower(rec.User + " " + rec.Action + " " + rec.ResourceName + " " + rec.ResourceType)
				if !strings.Contains(hay, q) {
					continue
				}
			}
			all = append(all, rec)
		}
		return nil
	})
	if err != nil {
		return ActivityListResult{}, err
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].Timestamp.After(all[j].Timestamp)
	})
	total := len(all)
	if f.Offset >= total {
		return ActivityListResult{Events: []ActivityRecord{}, Total: total}, nil
	}
	end := f.Offset + f.Limit
	if end > total {
		end = total
	}
	return ActivityListResult{Events: all[f.Offset:end], Total: total}, nil
}
