package postgres

import (
	"context"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
)

func (s *Store) PutTrustedCluster(rec metadata.TrustedCluster) error {
	if rec.AuthMode == "" {
		rec.AuthMode = metadata.TrustedClusterAuthMTLS
	}
	if rec.Status == "" {
		rec.Status = metadata.TrustedClusterStatusHealthy
	}
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO trusted_clusters (
			id, name, endpoint, auth_mode, peer_ca_fingerprint, status,
			cert_expires_at, last_rotated_at, next_rotation_at, safety_number,
			created_at, active, is_local
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (id) DO UPDATE SET
			name=$2, endpoint=$3, auth_mode=$4, peer_ca_fingerprint=$5, status=$6,
			cert_expires_at=$7, last_rotated_at=$8, next_rotation_at=$9, safety_number=$10,
			active=$12, is_local=$13`,
		rec.ID, rec.Name, rec.Endpoint, rec.AuthMode, rec.PeerCAFingerprint, rec.Status,
		timestamptzPtr(rec.CertExpiresAt), timestamptzPtr(rec.LastRotatedAt), timestamptzPtr(rec.NextRotationAt),
		rec.SafetyNumber, rec.CreatedAt, rec.Active, rec.IsLocal)
	return err
}

func (s *Store) GetTrustedCluster(id string) (metadata.TrustedCluster, error) {
	return s.scanTrustedCluster(`SELECT id, name, endpoint, auth_mode, peer_ca_fingerprint, status,
		cert_expires_at, last_rotated_at, next_rotation_at, safety_number, created_at, active, is_local
		FROM trusted_clusters WHERE id=$1`, id)
}

func (s *Store) ListTrustedClusters(activeOnly bool) ([]metadata.TrustedCluster, error) {
	q := `SELECT id, name, endpoint, auth_mode, peer_ca_fingerprint, status,
		cert_expires_at, last_rotated_at, next_rotation_at, safety_number, created_at, active, is_local
		FROM trusted_clusters`
	if activeOnly {
		q += ` WHERE active = TRUE`
	}
	q += ` ORDER BY created_at ASC`
	rows, err := s.pool.Query(context.Background(), q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.TrustedCluster
	for rows.Next() {
		rec, err := scanTrustedClusterRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) DeactivateTrustedCluster(id string) error {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE trusted_clusters SET active=FALSE, status=$2 WHERE id=$1`,
		id, metadata.TrustedClusterStatusRevoked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) PutClusterPairingSession(rec metadata.ClusterPairingSession) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO cluster_pairing_sessions (token_hash, expires_at, used_at, initiator_cluster_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (token_hash) DO UPDATE SET expires_at=$2, used_at=$3, initiator_cluster_id=$4`,
		rec.TokenHash, rec.ExpiresAt, timestamptzPtr(rec.UsedAt), rec.InitiatorClusterID)
	return err
}

func (s *Store) GetClusterPairingSession(tokenHash string) (metadata.ClusterPairingSession, error) {
	var rec metadata.ClusterPairingSession
	var usedAt *time.Time
	err := s.pool.QueryRow(context.Background(), `
		SELECT token_hash, expires_at, used_at, initiator_cluster_id
		FROM cluster_pairing_sessions WHERE token_hash=$1`, tokenHash).
		Scan(&rec.TokenHash, &rec.ExpiresAt, &usedAt, &rec.InitiatorClusterID)
	if err == pgx.ErrNoRows {
		return rec, metadata.ErrNotFound
	}
	if err != nil {
		return rec, err
	}
	rec.UsedAt = usedAt
	return rec, nil
}

func (s *Store) MarkClusterPairingSessionUsed(tokenHash string, usedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE cluster_pairing_sessions SET used_at=$2 WHERE token_hash=$1 AND used_at IS NULL`,
		tokenHash, usedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) PutClusterCertificate(rec metadata.ClusterCertificate) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO cluster_certificates (serial, cluster_id, role, not_before, not_after, revoked_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (serial) DO UPDATE SET cluster_id=$2, role=$3, not_before=$4, not_after=$5, revoked_at=$6`,
		rec.Serial, rec.ClusterID, rec.Role, rec.NotBefore, rec.NotAfter, timestamptzPtr(rec.RevokedAt))
	return err
}

func (s *Store) ListClusterCertificates(clusterID string) ([]metadata.ClusterCertificate, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT serial, cluster_id, role, not_before, not_after, revoked_at
		FROM cluster_certificates WHERE cluster_id=$1 ORDER BY not_before DESC`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.ClusterCertificate
	for rows.Next() {
		var rec metadata.ClusterCertificate
		var revokedAt *time.Time
		if err := rows.Scan(&rec.Serial, &rec.ClusterID, &rec.Role, &rec.NotBefore, &rec.NotAfter, &revokedAt); err != nil {
			return nil, err
		}
		rec.RevokedAt = revokedAt
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) RevokeClusterCertificate(serial string, revokedAt time.Time) error {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE cluster_certificates SET revoked_at=$2 WHERE serial=$1 AND revoked_at IS NULL`,
		serial, revokedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrNotFound
	}
	return nil
}

func (s *Store) ListRevokedClusterCertSerials() ([]string, error) {
	rows, err := s.pool.Query(context.Background(), `
		SELECT serial FROM cluster_certificates WHERE revoked_at IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var serial string
		if err := rows.Scan(&serial); err != nil {
			return nil, err
		}
		out = append(out, serial)
	}
	return out, rows.Err()
}

func (s *Store) scanTrustedCluster(q string, args ...any) (metadata.TrustedCluster, error) {
	row := s.pool.QueryRow(context.Background(), q, args...)
	return scanTrustedClusterRow(row)
}

type trustedClusterScanner interface {
	Scan(dest ...any) error
}

func scanTrustedClusterRow(row trustedClusterScanner) (metadata.TrustedCluster, error) {
	var rec metadata.TrustedCluster
	var certExp, lastRot, nextRot *time.Time
	err := row.Scan(
		&rec.ID, &rec.Name, &rec.Endpoint, &rec.AuthMode, &rec.PeerCAFingerprint, &rec.Status,
		&certExp, &lastRot, &nextRot, &rec.SafetyNumber, &rec.CreatedAt, &rec.Active, &rec.IsLocal,
	)
	if err == pgx.ErrNoRows {
		return rec, metadata.ErrNotFound
	}
	if err != nil {
		return rec, err
	}
	rec.CertExpiresAt = certExp
	rec.LastRotatedAt = lastRot
	rec.NextRotationAt = nextRot
	return rec, nil
}
