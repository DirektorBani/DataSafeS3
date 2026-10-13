package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Store) PutMFAActionToken(rec metadata.MFAActionToken) error {
	_, err := s.pool.Exec(context.Background(), `
		INSERT INTO mfa_action_tokens (token_hash, user_id, session_id, action, target, nonce, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (token_hash) DO NOTHING`,
		rec.Hash, rec.UserID, rec.SessionID, rec.Action, rec.Target, rec.Nonce, rec.ExpiresAt)
	return err
}

func (s *Store) ConsumeMFAActionToken(hash string, match metadata.MFATokenMatch, now time.Time) (metadata.MFAActionToken, error) {
	var rec metadata.MFAActionToken
	var used pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		UPDATE mfa_action_tokens
		SET used_at = $2
		WHERE token_hash = $1
		  AND used_at IS NULL
		  AND expires_at > $2
		  AND user_id = $3
		  AND session_id = $4
		  AND action = $5
		  AND target = $6
		RETURNING token_hash, user_id, session_id, action, target, nonce, expires_at, used_at`,
		hash, now.UTC(), match.UserID, match.SessionID, match.Action, match.Target).Scan(
		&rec.Hash, &rec.UserID, &rec.SessionID, &rec.Action, &rec.Target, &rec.Nonce, &rec.ExpiresAt, &used)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return metadata.MFAActionToken{}, metadata.ErrMFATokenInvalid
		}
		return metadata.MFAActionToken{}, err
	}
	rec.UsedAt = timePtr(used)
	return rec, nil
}

func (s *Store) PurgeMFAActionTokens(now time.Time) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM mfa_action_tokens WHERE expires_at < $1`, now.UTC())
	return err
}

func (s *Store) ClaimTOTPStep(userID string, step int64) (bool, error) {
	tag, err := s.pool.Exec(context.Background(), `
		UPDATE users SET totp_last_step = $2 WHERE id = $1 AND totp_last_step < $2`, userID, step)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) ClaimMFADeniedSlot(userID, action string, now time.Time) (bool, error) {
	var id string
	err := s.pool.QueryRow(context.Background(), `
		INSERT INTO mfa_denied_events (user_id, action, last_at) VALUES ($1,$2,$3)
		ON CONFLICT (user_id, action) DO UPDATE SET last_at = EXCLUDED.last_at
		WHERE mfa_denied_events.last_at < EXCLUDED.last_at - INTERVAL '1 minute'
		RETURNING user_id`, userID, action, now.UTC()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) NoteMFAStepUpFailure(userID string, now time.Time) (metadata.MFAStepUpState, error) {
	ctx := context.Background()
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return metadata.MFAStepUpState{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var rec struct {
		Failures    int
		WindowStart time.Time
		LockedUntil pgtype.Timestamptz
	}
	err = tx.QueryRow(ctx, `
		SELECT failures, window_start, locked_until FROM mfa_stepup_failures WHERE user_id=$1 FOR UPDATE`, userID).
		Scan(&rec.Failures, &rec.WindowStart, &rec.LockedUntil)
	stored := metadata.MFAFailRecord{Failures: rec.Failures, WindowStart: rec.WindowStart, LockedUntil: timePtr(rec.LockedUntil)}
	if errors.Is(err, pgx.ErrNoRows) {
		stored = metadata.MFAFailRecord{}
		err = nil
	}
	if err != nil {
		return metadata.MFAStepUpState{}, err
	}
	state, next := metadata.AdvanceMFAFailures(stored, now)
	var locked any
	if next.LockedUntil != nil {
		locked = *next.LockedUntil
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO mfa_stepup_failures (user_id, failures, window_start, locked_until)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (user_id) DO UPDATE SET failures=$2, window_start=$3, locked_until=$4`,
		userID, next.Failures, next.WindowStart, locked)
	if err != nil {
		return metadata.MFAStepUpState{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return metadata.MFAStepUpState{}, err
	}
	return state, nil
}

func (s *Store) MFAStepUpLocked(userID string, now time.Time) (metadata.MFAStepUpState, error) {
	var failures int
	var until pgtype.Timestamptz
	err := s.pool.QueryRow(context.Background(), `
		SELECT failures, locked_until FROM mfa_stepup_failures WHERE user_id=$1`, userID).Scan(&failures, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return metadata.MFAStepUpState{}, nil
	}
	if err != nil {
		return metadata.MFAStepUpState{}, err
	}
	t := timePtr(until)
	if t != nil && t.After(now) {
		return metadata.MFAStepUpState{Failures: failures, Locked: true, LockedUntil: *t}, nil
	}
	return metadata.MFAStepUpState{Failures: failures}, nil
}

func (s *Store) ResetTOTPStep(userID string) error {
	_, err := s.pool.Exec(context.Background(), `UPDATE users SET totp_last_step=0 WHERE id=$1`, userID)
	return err
}

func (s *Store) ClearMFAStepUpFailures(userID string) error {
	_, err := s.pool.Exec(context.Background(), `DELETE FROM mfa_stepup_failures WHERE user_id=$1`, userID)
	return err
}
