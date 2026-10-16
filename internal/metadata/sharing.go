package metadata

import (
	"encoding/json"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	SharingScopeGlobal = "global"
	SharingScopeTenant = "tenant"
	SharingScopeBucket = "bucket"

	// DefaultShareTTLSeconds is the built-in cap (7 days) until an administrator
	// disables the limit (-1) or sets a tighter value.
	DefaultShareTTLSeconds int64 = 7 * 24 * 3600
)

// SharingPolicy is one level of the global → tenant → bucket hierarchy.
// Nil pointers inherit the parent. A lower level can only tighten.
type SharingPolicy struct {
	Scope                 string    `json:"scope"`
	ScopeID               string    `json:"scope_id"`
	PublicAccessBlocked   *bool     `json:"public_access_blocked,omitempty"`
	ShareLinksAllowed     *bool     `json:"share_links_allowed,omitempty"`
	PresignAllowed        *bool     `json:"presign_allowed,omitempty"`
	MaxShareTTLSec        *int64    `json:"max_share_ttl_sec,omitempty"`
	MaxPresignTTLSec      *int64    `json:"max_presign_ttl_sec,omitempty"`
	SharePasswordRequired *bool     `json:"share_password_required,omitempty"`
	UpdatedAt             time.Time `json:"updated_at,omitempty"`
	UpdatedBy             string    `json:"updated_by,omitempty"`
}

// EffectiveSharing is the policy a request must enforce.
type EffectiveSharing struct {
	PublicAccessBlocked   bool   `json:"public_access_blocked"`
	ShareLinksAllowed     bool   `json:"share_links_allowed"`
	PresignAllowed        bool   `json:"presign_allowed"`
	MaxShareTTLSec        int64  `json:"max_share_ttl_sec"`
	MaxPresignTTLSec      int64  `json:"max_presign_ttl_sec"`
	SharePasswordRequired bool   `json:"share_password_required"`
	PublicAccessBlockedBy string `json:"public_access_blocked_by,omitempty"`
	ShareLinksBlockedBy   string `json:"share_links_blocked_by,omitempty"`
	PresignBlockedBy      string `json:"presign_blocked_by,omitempty"`
	ShareTTLBy            string `json:"share_ttl_by,omitempty"`
	PresignTTLBy          string `json:"presign_ttl_by,omitempty"`
	PasswordRequiredBy    string `json:"password_required_by,omitempty"`
}

// DefaultEffectiveSharing is the policy when nothing is configured.
func DefaultEffectiveSharing() EffectiveSharing {
	return EffectiveSharing{
		ShareLinksAllowed: true,
		PresignAllowed:    true,
		MaxShareTTLSec:    DefaultShareTTLSeconds,
		MaxPresignTTLSec:  DefaultShareTTLSeconds,
	}
}

// MergeSharing tightens base with one level. A negative TTL means "no cap"
// and is applied only when the parent has not already set a tighter cap.
func MergeSharing(base EffectiveSharing, level SharingPolicy, parentExplicitShare, parentExplicitPresign bool) (EffectiveSharing, bool, bool) {
	out := base
	shareExplicit := parentExplicitShare
	presignExplicit := parentExplicitPresign
	if level.PublicAccessBlocked != nil && *level.PublicAccessBlocked {
		out.PublicAccessBlocked = true
		out.PublicAccessBlockedBy = level.Scope
	}
	if level.ShareLinksAllowed != nil && !*level.ShareLinksAllowed {
		out.ShareLinksAllowed = false
		out.ShareLinksBlockedBy = level.Scope
	}
	if level.PresignAllowed != nil && !*level.PresignAllowed {
		out.PresignAllowed = false
		out.PresignBlockedBy = level.Scope
	}
	if level.SharePasswordRequired != nil && *level.SharePasswordRequired {
		out.SharePasswordRequired = true
		out.PasswordRequiredBy = level.Scope
	}
	if level.MaxShareTTLSec != nil {
		v := *level.MaxShareTTLSec
		if v < 0 {
			if !shareExplicit {
				out.MaxShareTTLSec = 0
				out.ShareTTLBy = level.Scope
				shareExplicit = true
			}
		} else if v > 0 && (out.MaxShareTTLSec == 0 || v < out.MaxShareTTLSec) {
			out.MaxShareTTLSec = v
			out.ShareTTLBy = level.Scope
			shareExplicit = true
		}
	}
	if level.MaxPresignTTLSec != nil {
		v := *level.MaxPresignTTLSec
		if v < 0 {
			if !presignExplicit {
				out.MaxPresignTTLSec = DefaultShareTTLSeconds
				out.PresignTTLBy = level.Scope
				presignExplicit = true
			}
		} else if v > 0 && v < out.MaxPresignTTLSec {
			out.MaxPresignTTLSec = v
			out.PresignTTLBy = level.Scope
			presignExplicit = true
		}
	}
	if out.MaxPresignTTLSec <= 0 || out.MaxPresignTTLSec > DefaultShareTTLSeconds {
		out.MaxPresignTTLSec = DefaultShareTTLSeconds
	}
	return out, shareExplicit, presignExplicit
}

// EffectiveSharingFrom walks global, then tenant, then bucket.
func EffectiveSharingFrom(global, tenant, bucket SharingPolicy) EffectiveSharing {
	eff := DefaultEffectiveSharing()
	var shareExplicit, presignExplicit bool
	eff, shareExplicit, presignExplicit = MergeSharing(eff, global, shareExplicit, presignExplicit)
	eff, shareExplicit, presignExplicit = MergeSharing(eff, tenant, shareExplicit, presignExplicit)
	eff, _, _ = MergeSharing(eff, bucket, shareExplicit, presignExplicit)
	return eff
}

// SharingLoosens reports why proposed cannot be saved under parent.
// parent is the effective policy of the levels above proposed.
// topLevel is true only for the global policy, which may disable the share TTL.
func SharingLoosens(parent EffectiveSharing, parentShareExplicit, parentPresignExplicit, topLevel bool, proposed SharingPolicy) string {
	if proposed.PublicAccessBlocked != nil && !*proposed.PublicAccessBlocked && parent.PublicAccessBlocked {
		return "public_access_blocked"
	}
	if proposed.ShareLinksAllowed != nil && *proposed.ShareLinksAllowed && !parent.ShareLinksAllowed {
		return "share_links_allowed"
	}
	if proposed.PresignAllowed != nil && *proposed.PresignAllowed && !parent.PresignAllowed {
		return "presign_allowed"
	}
	if proposed.SharePasswordRequired != nil && !*proposed.SharePasswordRequired && parent.SharePasswordRequired {
		return "share_password_required"
	}
	if proposed.MaxShareTTLSec != nil {
		v := *proposed.MaxShareTTLSec
		unlimitedParent := parentShareExplicit && parent.MaxShareTTLSec == 0
		if v < 0 && !topLevel && !unlimitedParent {
			return "max_share_ttl"
		}
		if v > 0 && parent.MaxShareTTLSec > 0 && v > parent.MaxShareTTLSec {
			return "max_share_ttl"
		}
	}
	if proposed.MaxPresignTTLSec != nil {
		v := *proposed.MaxPresignTTLSec
		if v < 0 && !topLevel {
			return "max_presign_ttl"
		}
		if v > 0 && v > parent.MaxPresignTTLSec {
			return "max_presign_ttl"
		}
		if v > DefaultShareTTLSeconds {
			return "max_presign_ttl"
		}
	}
	return ""
}

func sharingPolicyKey(scope, scopeID string) []byte {
	return []byte(scope + "\x00" + scopeID)
}

func (s *Store) PutSharingPolicy(rec SharingPolicy) error {
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = time.Now().UTC()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("sharing_policies"))
		if err != nil {
			return err
		}
		data, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put(sharingPolicyKey(rec.Scope, rec.ScopeID), data)
	})
}

func (s *Store) GetSharingPolicy(scope, scopeID string) (SharingPolicy, error) {
	var rec SharingPolicy
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("sharing_policies"))
		if b == nil {
			return ErrNotFound
		}
		data := b.Get(sharingPolicyKey(scope, scopeID))
		if data == nil {
			return ErrNotFound
		}
		return json.Unmarshal(data, &rec)
	})
	return rec, err
}

func (s *Store) TouchSharePasswordFailure(id string, failures int, at *time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("shared_links"))
		data := b.Get([]byte(id))
		if data == nil {
			return ErrNotFound
		}
		var rec SharedLinkRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return err
		}
		rec.PasswordFailures = failures
		rec.PasswordFailureAt = at
		updated, err := json.Marshal(rec)
		if err != nil {
			return err
		}
		return b.Put([]byte(id), updated)
	})
}
