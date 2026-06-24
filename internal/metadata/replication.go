package metadata

import (
	"encoding/json"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	ReplEventPut    = "put"
	ReplEventDelete = "delete"
	ReplEventCopy   = "copy"

	ReplTaskPending   = "pending"
	ReplTaskCompleted = "completed"
	ReplTaskFailed    = "failed"
)

type ReplicationTask struct {
	ID           string     `json:"id"`
	RuleID       string     `json:"rule_id"`
	Event        string     `json:"event"`
	SourceBucket string     `json:"source_bucket"`
	Key          string     `json:"key"`
	Status       string     `json:"status"`
	Attempts     int        `json:"attempts"`
	Bytes        int64      `json:"bytes"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	NextAttempt  time.Time  `json:"next_attempt,omitempty"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
}

type GatewayStats struct {
	PendingCount        int       `json:"pending_count"`
	BytesReplicated     int64     `json:"bytes_replicated"`
	ReplicationErrors   int       `json:"replication_errors"`
	OldestPending       time.Time `json:"oldest_pending,omitempty"`
	LastProcessedAt     time.Time `json:"last_processed_at,omitempty"`
	TasksCompletedTotal int       `json:"tasks_completed_total"`
}

type ReplicationErrorRecord struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id,omitempty"`
	RuleID       string    `json:"rule_id"`
	Event        string    `json:"event"`
	SourceBucket string    `json:"source_bucket"`
	Key          string    `json:"key"`
	Message      string    `json:"message"`
	CreatedAt    time.Time `json:"created_at"`
}

const maxReplicationErrors = 50

func (s *Store) initReplicationBuckets(tx *bolt.Tx) error {
	for _, name := range []string{"replication_tasks", "gateway_stats", "replication_errors"} {
		if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PutReplicationTask(rec ReplicationTask) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("replication_tasks")).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetReplicationTask(id string) (ReplicationTask, error) {
	var rec ReplicationTask
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("replication_tasks")).Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListReplicationTasks(status string, limit int) ([]ReplicationTask, error) {
	var out []ReplicationTask
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("replication_tasks")).ForEach(func(_, v []byte) error {
			var rec ReplicationTask
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if status != "" && rec.Status != status {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) ListDueReplicationTasks(limit int, now time.Time) ([]ReplicationTask, error) {
	var out []ReplicationTask
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("replication_tasks")).ForEach(func(_, v []byte) error {
			var rec ReplicationTask
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.Status != ReplTaskPending {
				return nil
			}
			if !rec.NextAttempt.IsZero() && rec.NextAttempt.After(now) {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) CountPendingReplicationTasks() (int, time.Time, error) {
	var count int
	var oldest time.Time
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte("replication_tasks")).ForEach(func(_, v []byte) error {
			var rec ReplicationTask
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.Status != ReplTaskPending {
				return nil
			}
			count++
			if oldest.IsZero() || rec.CreatedAt.Before(oldest) {
				oldest = rec.CreatedAt
			}
			return nil
		})
	})
	return count, oldest, err
}

func (s *Store) GetGatewayStats() (GatewayStats, error) {
	var stats GatewayStats
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte("gateway_stats")).Get([]byte("global"))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &stats)
	})
	if err != nil {
		return stats, err
	}
	pending, oldest, err := s.CountPendingReplicationTasks()
	if err != nil {
		return stats, err
	}
	stats.PendingCount = pending
	stats.OldestPending = oldest
	return stats, nil
}

func (s *Store) PutGatewayStats(stats GatewayStats) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("gateway_stats")).Put([]byte("global"), data)
	})
}

func (s *Store) UpdateGatewayStats(fn func(*GatewayStats)) error {
	stats, err := s.GetGatewayStats()
	if err != nil {
		return err
	}
	fn(&stats)
	pending, oldest, err := s.CountPendingReplicationTasks()
	if err != nil {
		return err
	}
	stats.PendingCount = pending
	stats.OldestPending = oldest
	return s.PutGatewayStats(stats)
}

func (s *Store) ListReplicationRulesForBucket(bucket string) ([]ReplicationRule, error) {
	all, err := s.ListReplicationRules()
	if err != nil {
		return nil, err
	}
	var out []ReplicationRule
	for _, r := range all {
		if r.Enabled && r.SourceBucket == bucket {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) AddReplicationError(rec ReplicationErrorRecord) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("replication_errors"))
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		key := []byte(rec.CreatedAt.Format(time.RFC3339Nano) + "\x00" + rec.ID)
		if err := b.Put(key, data); err != nil {
			return err
		}
		var keys [][]byte
		_ = b.ForEach(func(k, _ []byte) error {
			keys = append(keys, append([]byte(nil), k...))
			return nil
		})
		if len(keys) <= maxReplicationErrors {
			return nil
		}
		sort.Slice(keys, func(i, j int) bool { return string(keys[i]) > string(keys[j]) })
		for _, k := range keys[maxReplicationErrors:] {
			_ = b.Delete(k)
		}
		return nil
	})
}

func (s *Store) ListReplicationErrors(limit int) ([]ReplicationErrorRecord, error) {
	var out []ReplicationErrorRecord
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("replication_errors"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec ReplicationErrorRecord
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) ClearReplicationErrors() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte("replication_errors")); err != nil && err != bolt.ErrBucketNotFound {
			return err
		}
		if _, err := tx.CreateBucketIfNotExists([]byte("replication_errors")); err != nil {
			return err
		}
		tasks := tx.Bucket([]byte("replication_tasks"))
		var toDelete [][]byte
		_ = tasks.ForEach(func(k, v []byte) error {
			var rec ReplicationTask
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.Status == ReplTaskFailed {
				toDelete = append(toDelete, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range toDelete {
			_ = tasks.Delete(k)
		}
		statsData := tx.Bucket([]byte("gateway_stats")).Get([]byte("global"))
		if statsData == nil {
			return nil
		}
		var stats GatewayStats
		if err := json.Unmarshal(statsData, &stats); err != nil {
			return err
		}
		stats.ReplicationErrors = 0
		updated, err := json.Marshal(stats)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte("gateway_stats")).Put([]byte("global"), updated)
	})
}

func (s *Store) RetryFailedReplicationTasks() (int, error) {
	now := time.Now().UTC()
	var count int
	err := s.db.Update(func(tx *bolt.Tx) error {
		tasks := tx.Bucket([]byte("replication_tasks"))
		return tasks.ForEach(func(k, v []byte) error {
			var rec ReplicationTask
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.Status != ReplTaskFailed {
				return nil
			}
			rec.Status = ReplTaskPending
			rec.Attempts = 0
			rec.Error = ""
			rec.NextAttempt = now
			rec.ProcessedAt = nil
			data, err := json.Marshal(rec)
			if err != nil {
				return err
			}
			if err := tasks.Put(k, data); err != nil {
				return err
			}
			count++
			return nil
		})
	})
	return count, err
}

func (s *Store) CountBrokenReplicationRules() (int, error) {
	rules, err := s.ListReplicationRules()
	if err != nil {
		return 0, err
	}
	var broken int
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if _, err := s.GetGatewayConnection(rule.DestConnection); err != nil {
			broken++
		}
	}
	return broken, nil
}
