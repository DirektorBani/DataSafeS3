package metadata

import (
	"encoding/json"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	boltTrustedClusters        = "trusted_clusters"
	boltClusterPairingSessions = "cluster_pairing_sessions"
	boltClusterCertificates    = "cluster_certificates"
)

func (s *Store) initTrustedClusterBuckets(tx *bolt.Tx) error {
	for _, name := range []string{boltTrustedClusters, boltClusterPairingSessions, boltClusterCertificates} {
		if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PutTrustedCluster(rec TrustedCluster) error {
	if rec.AuthMode == "" {
		rec.AuthMode = TrustedClusterAuthMTLS
	}
	if rec.Status == "" {
		rec.Status = TrustedClusterStatusHealthy
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initTrustedClusterBuckets(tx); err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(boltTrustedClusters)).Put([]byte(rec.ID), data)
	})
}

func (s *Store) GetTrustedCluster(id string) (TrustedCluster, error) {
	var rec TrustedCluster
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltTrustedClusters))
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

func (s *Store) ListTrustedClusters(activeOnly bool) ([]TrustedCluster, error) {
	var out []TrustedCluster
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltTrustedClusters))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec TrustedCluster
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if activeOnly && !rec.Active {
				return nil
			}
			out = append(out, rec)
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, err
}

func (s *Store) DeactivateTrustedCluster(id string) error {
	rec, err := s.GetTrustedCluster(id)
	if err != nil {
		return err
	}
	rec.Active = false
	rec.Status = TrustedClusterStatusRevoked
	return s.PutTrustedCluster(rec)
}

func (s *Store) PutClusterPairingSession(rec ClusterPairingSession) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initTrustedClusterBuckets(tx); err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(boltClusterPairingSessions)).Put([]byte(rec.TokenHash), data)
	})
}

func (s *Store) GetClusterPairingSession(tokenHash string) (ClusterPairingSession, error) {
	var rec ClusterPairingSession
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltClusterPairingSessions))
		if b == nil {
			return ErrNotFound
		}
		data := b.Get([]byte(tokenHash))
		if data == nil {
			return ErrNotFound
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.TokenHash = tokenHash
		return nil
	})
	return rec, err
}

func (s *Store) MarkClusterPairingSessionUsed(tokenHash string, usedAt time.Time) error {
	rec, err := s.GetClusterPairingSession(tokenHash)
	if err != nil {
		return err
	}
	if rec.UsedAt != nil {
		return ErrNotFound
	}
	rec.UsedAt = &usedAt
	return s.PutClusterPairingSession(rec)
}

func (s *Store) PutClusterCertificate(rec ClusterCertificate) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := s.initTrustedClusterBuckets(tx); err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return tx.Bucket([]byte(boltClusterCertificates)).Put([]byte(rec.Serial), data)
	})
}

func (s *Store) ListClusterCertificates(clusterID string) ([]ClusterCertificate, error) {
	var out []ClusterCertificate
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltClusterCertificates))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec ClusterCertificate
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.ClusterID == clusterID {
				out = append(out, rec)
			}
			return nil
		})
	})
	sort.Slice(out, func(i, j int) bool { return out[i].NotBefore.After(out[j].NotBefore) })
	return out, err
}

func (s *Store) RevokeClusterCertificate(serial string, revokedAt time.Time) error {
	var rec ClusterCertificate
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte(boltClusterCertificates)).Get([]byte(serial))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	if err != nil {
		return err
	}
	if rec.RevokedAt != nil {
		return ErrNotFound
	}
	rec.RevokedAt = &revokedAt
	return s.PutClusterCertificate(rec)
}

func (s *Store) ListRevokedClusterCertSerials() ([]string, error) {
	var out []string
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(boltClusterCertificates))
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var rec ClusterCertificate
			if err := json.Unmarshal(v, &rec); err != nil {
				return err
			}
			if rec.RevokedAt != nil {
				out = append(out, rec.Serial)
			}
			return nil
		})
	})
	return out, err
}
