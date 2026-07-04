package metadata

import (
	"encoding/json"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	boltSiteReplPeers = "site_replication_peers"
	boltSiteReplRules = "site_replication_rules"
	boltSiteReplTasks = "site_replication_tasks"
)

func (s *Store) initSiteReplBuckets(tx *bolt.Tx) error {
	for _, name := range []string{boltSiteReplPeers, boltSiteReplRules, boltSiteReplTasks} {
		if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PutSiteReplicationPeer(rec SiteReplicationPeer) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initSiteReplBuckets(tx); err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(boltSiteReplPeers)).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetSiteReplicationPeer(id string) (SiteReplicationPeer, error) {
	var rec SiteReplicationPeer
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltSiteReplPeers))
		if b == nil {
			return ErrNotFound
		}
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) ListSiteReplicationPeers() ([]SiteReplicationPeer, error) {
	var out []SiteReplicationPeer
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltSiteReplPeers))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec SiteReplicationPeer
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, err
}

func (s *Store) DeleteSiteReplicationPeer(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initSiteReplBuckets(tx); err != nil {
			return err
		}
		b := tx.Bucket([]byte(boltSiteReplPeers))
		if b == nil || b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) PutSiteReplicationRule(rec SiteReplicationRule) error {
	if rec.Direction == "" {
		rec.Direction = "one-way"
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initSiteReplBuckets(tx); err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(boltSiteReplRules)).Put([]byte(rec.ID), data)
	})
}

func (s *Store) ListSiteReplicationRules() ([]SiteReplicationRule, error) {
	var out []SiteReplicationRule
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltSiteReplRules))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec SiteReplicationRule
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, err
}

func (s *Store) DeleteSiteReplicationRule(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initSiteReplBuckets(tx); err != nil {
			return err
		}
		b := tx.Bucket([]byte(boltSiteReplRules))
		if b == nil || b.Get([]byte(id)) == nil {
			return ErrNotFound
		}
		return b.Delete([]byte(id))
	})
}

func (s *Store) ListSiteReplicationRulesForBucket(bucket string) ([]SiteReplicationRule, error) {
	all, err := s.ListSiteReplicationRules()
	if err != nil {
		return nil, err
	}
	var out []SiteReplicationRule
	for _, r := range all {
		if r.Enabled && r.SourceBucket == bucket {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) ListSiteReplicationRulesForTrustedCluster(clusterID string) ([]SiteReplicationRule, error) {
	all, err := s.ListSiteReplicationRules()
	if err != nil {
		return nil, err
	}
	var out []SiteReplicationRule
	for _, r := range all {
		if r.TrustedClusterID == clusterID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Store) PutSiteReplicationTask(rec SiteReplicationTask) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initSiteReplBuckets(tx); err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(boltSiteReplTasks)).Put([]byte(rec.ID), data)
	})
}

func (s *Store) ListDueSiteReplicationTasks(limit int, now time.Time) ([]SiteReplicationTask, error) {
	all, err := s.listSiteReplicationTasks()
	if err != nil {
		return nil, err
	}
	var out []SiteReplicationTask
	for _, rec := range all {
		if rec.Status != SiteReplTaskPending {
			continue
		}
		if !rec.NextAttempt.IsZero() && rec.NextAttempt.After(now) {
			continue
		}
		out = append(out, rec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) listSiteReplicationTasks() ([]SiteReplicationTask, error) {
	var out []SiteReplicationTask
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltSiteReplTasks))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec SiteReplicationTask
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			out = append(out, rec)
			return nil
		})
	})
	return out, err
}

func (s *Store) CountPendingSiteReplicationTasks() (int, error) {
	all, err := s.listSiteReplicationTasks()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, rec := range all {
		if rec.Status == SiteReplTaskPending {
			n++
		}
	}
	return n, nil
}

func (s *Store) SiteReplicationStatus() (SiteReplicationStatus, error) {
	pending, err := s.CountPendingSiteReplicationTasks()
	if err != nil {
		return SiteReplicationStatus{}, err
	}
	st := SiteReplicationStatus{PendingCount: pending}
	all, err := s.listSiteReplicationTasks()
	if err != nil {
		return st, err
	}
	for _, rec := range all {
		if rec.Status != SiteReplTaskDone && rec.Status != SiteReplTaskFailed {
			continue
		}
		if rec.ProcessedAt == nil {
			continue
		}
		if st.LastProcessedAt.IsZero() || rec.ProcessedAt.After(st.LastProcessedAt) {
			st.LastProcessedAt = *rec.ProcessedAt
			st.LastError = rec.Error
		}
	}
	if !st.LastProcessedAt.IsZero() {
		st.LagSeconds = time.Since(st.LastProcessedAt).Seconds()
	}
	return st, nil
}
