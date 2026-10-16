package metadata

import (
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	mfaTokenBucket  = "mfa_action_tokens"
	mfaDeniedBucket = "mfa_denied_events"
	mfaFailBucket   = "mfa_stepup_failures"
)

// ErrMFATokenInvalid means the action token is missing, used, expired, or bound
// to a different user, session, action, or target.
var ErrMFATokenInvalid = errors.New("mfa token invalid")

// MFAActionToken is the stored hash of a one-time step-up token.
type MFAActionToken struct {
	Hash      string     `json:"hash"`
	UserID    string     `json:"user_id"`
	SessionID string     `json:"session_id"`
	Action    string     `json:"action"`
	Target    string     `json:"target"`
	Nonce     string     `json:"nonce"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
}

// MFATokenMatch is the binding checked when a token is consumed.
type MFATokenMatch struct {
	UserID    string
	SessionID string
	Action    string
	Target    string
}

// MFAStepUpState is the failure window for one user.
type MFAStepUpState struct {
	Failures    int
	Locked      bool
	LockedUntil time.Time
}

// MFAFailRecord is the persisted step-up failure window for one user.
type MFAFailRecord struct {
	Failures    int        `json:"failures"`
	WindowStart time.Time  `json:"window_start"`
	LockedUntil *time.Time `json:"locked_until,omitempty"`
}

func (s *Store) PutMFAActionToken(rec MFAActionToken) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(mfaTokenBucket))
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(rec.Hash), data)
	})
}

func (s *Store) ConsumeMFAActionToken(hash string, match MFATokenMatch, now time.Time) (MFAActionToken, error) {
	var out MFAActionToken
	err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(mfaTokenBucket))
		if err != nil {
			return err
		}
		raw := b.Get([]byte(hash))
		if raw == nil {
			return ErrMFATokenInvalid
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		if out.UsedAt != nil || !out.ExpiresAt.After(now) ||
			out.UserID != match.UserID || out.SessionID != match.SessionID ||
			out.Action != match.Action || out.Target != match.Target {
			return ErrMFATokenInvalid
		}
		used := now.UTC()
		out.UsedAt = &used
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return b.Put([]byte(hash), data)
	})
	if err != nil {
		return MFAActionToken{}, err
	}
	return out, nil
}

func (s *Store) PurgeMFAActionTokens(now time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(mfaTokenBucket))
		if b == nil {
			return nil
		}
		var drop [][]byte
		_ = b.ForEach(func(k, v []byte) error {
			var rec MFAActionToken
			if json.Unmarshal(v, &rec) != nil || !rec.ExpiresAt.After(now) {
				drop = append(drop, append([]byte(nil), k...))
			}
			return nil
		})
		for _, k := range drop {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ClaimTOTPStep(userID string, step int64) (bool, error) {
	claimed := false
	err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		raw := b.Get([]byte(userID))
		if raw == nil {
			return ErrUserNotFound
		}
		var rec UserRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		if rec.TOTPLastStep >= step {
			return nil
		}
		rec.TOTPLastStep = step
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		claimed = true
		return b.Put([]byte(userID), data)
	})
	return claimed, err
}

func (s *Store) ClaimMFADeniedSlot(userID, action string, now time.Time) (bool, error) {
	key := userID + "\x00" + action
	claimed := false
	err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(mfaDeniedBucket))
		if err != nil {
			return err
		}
		if raw := b.Get([]byte(key)); raw != nil {
			prev, err := time.Parse(time.RFC3339Nano, string(raw))
			if err == nil && now.Sub(prev) < time.Minute {
				return nil
			}
		}
		claimed = true
		return b.Put([]byte(key), []byte(now.UTC().Format(time.RFC3339Nano)))
	})
	return claimed, err
}

func (s *Store) NoteMFAStepUpFailure(userID string, now time.Time) (MFAStepUpState, error) {
	var state MFAStepUpState
	err := s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(mfaFailBucket))
		if err != nil {
			return err
		}
		var rec MFAFailRecord
		if raw := b.Get([]byte(userID)); raw != nil {
			_ = json.Unmarshal(raw, &rec)
		}
		state, rec = AdvanceMFAFailures(rec, now)
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(userID), data)
	})
	return state, err
}

func (s *Store) MFAStepUpLocked(userID string, now time.Time) (MFAStepUpState, error) {
	var state MFAStepUpState
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(mfaFailBucket))
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(userID))
		if raw == nil {
			return nil
		}
		var rec MFAFailRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		if rec.LockedUntil != nil && rec.LockedUntil.After(now) {
			state = MFAStepUpState{Failures: rec.Failures, Locked: true, LockedUntil: *rec.LockedUntil}
		}
		return nil
	})
	return state, err
}

func (s *Store) ResetTOTPStep(userID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("users"))
		raw := b.Get([]byte(userID))
		if raw == nil {
			return ErrUserNotFound
		}
		var rec UserRecord
		if err := json.Unmarshal(raw, &rec); err != nil {
			return err
		}
		rec.TOTPLastStep = 0
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(userID), data)
	})
}

func (s *Store) ClearMFAStepUpFailures(userID string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(mfaFailBucket))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(userID))
	})
}

// AdvanceMFAFailures applies the 5-failures / 5-minute lock to a stored window.
func AdvanceMFAFailures(rec MFAFailRecord, now time.Time) (MFAStepUpState, MFAFailRecord) {
	now = now.UTC()
	if rec.LockedUntil != nil && rec.LockedUntil.After(now) {
		return MFAStepUpState{Failures: rec.Failures, Locked: true, LockedUntil: *rec.LockedUntil}, rec
	}
	if rec.WindowStart.IsZero() || now.Sub(rec.WindowStart) > 5*time.Minute {
		rec.Failures = 1
		rec.WindowStart = now
		rec.LockedUntil = nil
		return MFAStepUpState{Failures: 1}, rec
	}
	rec.Failures++
	if rec.Failures >= 5 {
		until := now.Add(5 * time.Minute)
		rec.LockedUntil = &until
		return MFAStepUpState{Failures: rec.Failures, Locked: true, LockedUntil: until}, rec
	}
	return MFAStepUpState{Failures: rec.Failures}, rec
}
