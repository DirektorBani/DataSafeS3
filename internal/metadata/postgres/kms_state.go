package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetKMSState() (metadata.KMSState, error) {
	var st metadata.KMSState
	err := s.pool.QueryRow(context.Background(), `
		SELECT fingerprint, legacy_sse_objects, initialized FROM kms_state WHERE id='local'`).
		Scan(&st.Fingerprint, &st.LegacySSEObjects, &st.Initialized)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return metadata.KMSState{}, nil
		}
		return metadata.KMSState{}, err
	}
	return st, nil
}

func (s *Store) InitKMSState(st metadata.KMSState) (metadata.KMSState, error) {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO kms_state (id, fingerprint, legacy_sse_objects, initialized)
		VALUES ('local', $1, $2, TRUE)
		ON CONFLICT (id) DO NOTHING`, st.Fingerprint, st.LegacySSEObjects)
	if err != nil {
		return metadata.KMSState{}, err
	}
	return s.GetKMSState()
}

func (s *Store) PutKMSState(st metadata.KMSState) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO kms_state (id, fingerprint, legacy_sse_objects, initialized)
		VALUES ('local', $1, $2, TRUE)
		ON CONFLICT (id) DO UPDATE SET
			fingerprint = EXCLUDED.fingerprint,
			legacy_sse_objects = EXCLUDED.legacy_sse_objects,
			initialized = TRUE`, st.Fingerprint, st.LegacySSEObjects)
	return err
}

func (s *Store) PutKMSKey(rec metadata.KMSKeyRecord) error {
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO kms_keys (id, provider, status, key_version, generation, tenant_id, bucket_key, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			provider=EXCLUDED.provider, status=EXCLUDED.status, key_version=EXCLUDED.key_version,
			generation=EXCLUDED.generation, tenant_id=EXCLUDED.tenant_id, bucket_key=EXCLUDED.bucket_key,
			updated_at=EXCLUDED.updated_at`,
		rec.ID, rec.Provider, rec.Status, rec.Version, rec.Generation, rec.TenantID, rec.BucketKey, rec.UpdatedAt)
	return err
}

func (s *Store) GetKMSKey(id string) (metadata.KMSKeyRecord, error) {
	var rec metadata.KMSKeyRecord
	err := s.pool.QueryRow(context.Background(), `
		SELECT id, provider, status, key_version, generation, COALESCE(tenant_id,''), COALESCE(bucket_key,''), updated_at
		FROM kms_keys WHERE id=$1`, id).Scan(
		&rec.ID, &rec.Provider, &rec.Status, &rec.Version, &rec.Generation, &rec.TenantID, &rec.BucketKey, &rec.UpdatedAt)
	if err != nil {
		return rec, metadata.ErrNotFound
	}
	return rec, nil
}

func (s *Store) PutBucketEncryption(storageKey string, enc metadata.BucketEncryption) error {
	if enc.Algorithm == "" {
		_, err := s.pool.Exec(context.Background(), `DELETE FROM bucket_encryption WHERE storage_key=$1`, storageKey)
		return err
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO bucket_encryption (storage_key, algorithm, kms_key_id, provider, key_status)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (storage_key) DO UPDATE SET
			algorithm=EXCLUDED.algorithm, kms_key_id=EXCLUDED.kms_key_id,
			provider=EXCLUDED.provider, key_status=EXCLUDED.key_status`,
		storageKey, enc.Algorithm, enc.KeyID, enc.Provider, enc.Status)
	return err
}

func (s *Store) GetBucketEncryption(storageKey string) (metadata.BucketEncryption, error) {
	var enc metadata.BucketEncryption
	err := s.pool.QueryRow(context.Background(), `
		SELECT storage_key, algorithm, COALESCE(kms_key_id,''), COALESCE(provider,''), COALESCE(key_status,'')
		FROM bucket_encryption WHERE storage_key=$1`, storageKey).Scan(
		&enc.StorageKey, &enc.Algorithm, &enc.KeyID, &enc.Provider, &enc.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return metadata.BucketEncryption{StorageKey: storageKey}, nil
		}
		return metadata.BucketEncryption{}, err
	}
	return enc, nil
}

func (s *Store) DeleteBucketEncryption(storageKey string) error {
	return s.PutBucketEncryption(storageKey, metadata.BucketEncryption{StorageKey: storageKey})
}

func (s *Store) applyBucketEncryption(rec *metadata.BucketRecord) {
	if rec == nil || rec.EffectiveStorageKey() == "" {
		return
	}
	enc, err := s.GetBucketEncryption(rec.EffectiveStorageKey())
	if err != nil || enc.Algorithm == "" {
		return
	}
	rec.DefaultEncryption = enc.Algorithm
	rec.KMSMasterKeyID = enc.KeyID
	rec.EncryptionProvider = enc.Provider
	rec.KMSKeyStatus = enc.Status
}

func (s *Store) UpsertNodeHeartbeat(nodeID, version string, at time.Time) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO cluster_node_heartbeats (node_id, version, seen_at)
		VALUES ($1,$2,$3)
		ON CONFLICT (node_id) DO UPDATE SET version=EXCLUDED.version, seen_at=EXCLUDED.seen_at`,
		nodeID, version, at.UTC())
	return err
}

func (s *Store) ListNodeHeartbeats(since time.Time) ([]metadata.NodeHeartbeat, error) {
	if since.IsZero() {
		since = time.Unix(0, 0).UTC()
	}
	rows, err := s.pool.Query(context.Background(), `
		SELECT node_id, version, seen_at FROM cluster_node_heartbeats
		WHERE seen_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.NodeHeartbeat
	for rows.Next() {
		var rec metadata.NodeHeartbeat
		if err := rows.Scan(&rec.NodeID, &rec.Version, &rec.SeenAt); err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}
