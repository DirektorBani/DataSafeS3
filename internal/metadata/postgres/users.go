package postgres

import (
	"context"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5/pgtype"
)

const userSelect = `
	SELECT id, username, email, password_hash, role, status, COALESCE(tenant_id,''), COALESCE(team_id,''), mfa_enabled,
		COALESCE(totp_secret,''), recovery_codes, COALESCE(auth_source,''), max_size_bytes, max_objects,
		last_login, COALESCE(locale,''), COALESCE(webauthn_credentials,''), created_at FROM users`

func (s *Store) PutUser(rec metadata.UserRecord) error {
	ctx := context.Background()
	var exists bool
	_ = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 OR username=$2)`, rec.ID, rec.Username).Scan(&exists)
	if exists {
		return metadata.ErrUserExists
	}
	codes, _ := marshalJSON(rec.RecoveryCodes)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO users (id, username, email, password_hash, role, status, tenant_id, team_id, mfa_enabled,
			totp_secret, recovery_codes, auth_source, max_size_bytes, max_objects, last_login, locale, webauthn_credentials, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`,
		rec.ID, rec.Username, rec.Email, rec.PasswordHash, rec.Role, rec.Status, rec.TenantID, optionalText(rec.TeamID),
		rec.MFAEnabled, rec.TOTPSecret, codes, rec.AuthSource, rec.MaxSizeBytes, rec.MaxObjects,
		timestamptzPtr(rec.LastLogin), rec.Locale, rec.WebAuthnCredentials, rec.CreatedAt)
	return err
}

func (s *Store) GetUser(id string) (metadata.UserRecord, error) {
	return s.scanUser(s.pool.QueryRow(context.Background(), userSelect+` WHERE id=$1`, id))
}

func (s *Store) GetUserByUsername(username string) (metadata.UserRecord, error) {
	return s.scanUser(s.pool.QueryRow(context.Background(), userSelect+` WHERE username=$1`, username))
}

func (s *Store) scanUser(row interface{ Scan(dest ...any) error }) (metadata.UserRecord, error) {
	var rec metadata.UserRecord
	var codes []byte
	var lastLogin pgtype.Timestamptz
	err := row.Scan(&rec.ID, &rec.Username, &rec.Email, &rec.PasswordHash, &rec.Role, &rec.Status,
		&rec.TenantID, &rec.TeamID, &rec.MFAEnabled, &rec.TOTPSecret, &codes, &rec.AuthSource,
		&rec.MaxSizeBytes, &rec.MaxObjects, &lastLogin, &rec.Locale, &rec.WebAuthnCredentials, &rec.CreatedAt)
	if err != nil {
		return rec, metadata.ErrUserNotFound
	}
	_ = unmarshalJSON(codes, &rec.RecoveryCodes)
	rec.LastLogin = timePtr(lastLogin)
	return rec, nil
}

func (s *Store) UpdateUser(rec metadata.UserRecord) error {
	codes, _ := marshalJSON(rec.RecoveryCodes)
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE users SET email=$2, password_hash=$3, role=$4, status=$5, tenant_id=$6, team_id=$7, mfa_enabled=$8,
			totp_secret=$9, recovery_codes=$10, auth_source=$11, max_size_bytes=$12, max_objects=$13,
			last_login=$14, locale=$15, webauthn_credentials=$16 WHERE id=$1`,
		rec.ID, rec.Email, rec.PasswordHash, rec.Role, rec.Status, rec.TenantID, optionalText(rec.TeamID), rec.MFAEnabled,
		rec.TOTPSecret, codes, rec.AuthSource, rec.MaxSizeBytes, rec.MaxObjects, timestamptzPtr(rec.LastLogin), rec.Locale, rec.WebAuthnCredentials)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrUserNotFound
	}
	return nil
}

func (s *Store) DeleteUser(id string) error {
	ctx := context.Background()
	rec, err := s.GetUser(id)
	if err != nil {
		return err
	}
	if rec.Role == metadata.RoleAdministrator {
		var admins int
		_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE role=$1`, metadata.RoleAdministrator).Scan(&admins)
		if admins <= 1 {
			return metadata.ErrLastAdmin
		}
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return metadata.ErrUserNotFound
	}
	return nil
}

func (s *Store) ListUsers() ([]metadata.UserRecord, error) {
	rows, err := s.pool.Query(context.Background(), userSelect+` ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metadata.UserRecord
	for rows.Next() {
		rec, err := s.scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}
