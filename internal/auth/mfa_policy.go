package auth

import (
	"strings"
	"time"
)

const (
	// CodeMFARequired is returned when the caller has no accepted factor after grace.
	CodeMFARequired = "mfa_required"
	// CodeStepUp is returned when a fresh action token is required.
	CodeStepUp = "step_up_required"
)

// DefaultOIDCAMR is the RFC 8176 list accepted when the operator has not set one.
var DefaultOIDCAMR = []string{"mfa", "otp", "hwk", "swk", "fpt"}

// MFAPolicy is the effective bucket-admin 2FA policy.
type MFAPolicy struct {
	Enabled           bool
	GraceDays         int
	ApplyTenantAdmins bool
	ApplyGlobalAdmins bool
	EnabledAt         time.Time
}

// SubjectInput describes why a user may be covered by the policy.
type SubjectInput struct {
	Role             string
	BucketAdmin      bool
	TenantAdmin      bool
	BucketAdminSince *time.Time
	SubjectSince     *time.Time
}

// FactorState is what the user can prove.
type FactorState struct {
	TOTP     bool
	WebAuthn bool
	IdPMFA   bool
}

func (f FactorState) HasFactor() bool {
	return f.TOTP || f.WebAuthn || f.IdPMFA
}

func (f FactorState) HasLocalFactor() bool {
	return f.TOTP || f.WebAuthn
}

// Decision is the gate for one dangerous or bucket-admin action.
type Decision struct {
	Subject        bool
	InGrace        bool
	GraceUntil     time.Time
	HasFactor      bool
	HasLocalFactor bool
	// Code is empty when the action may proceed, CodeMFARequired, or CodeStepUp.
	Code string
}

// Decide applies the policy. dangerous is true for actions in DangerousActions.
// During grace a user without a local factor may proceed (banner only). A user
// who already has a local factor must still step up for a dangerous action.
// After grace, no accepted factor yields mfa_required. An IdP factor satisfies
// "has 2FA" but dangerous actions still need a local DataSafe factor.
func Decide(p MFAPolicy, sub SubjectInput, f FactorState, dangerous bool, now time.Time) Decision {
	d := Decision{
		HasFactor:      f.HasFactor(),
		HasLocalFactor: f.HasLocalFactor(),
	}
	if !p.Enabled {
		return d
	}
	d.Subject = subject(p, sub)
	if !d.Subject {
		return d
	}
	start := p.EnabledAt
	if start.IsZero() {
		start = now
	}
	if sub.BucketAdmin && sub.BucketAdminSince != nil && sub.BucketAdminSince.After(start) {
		start = *sub.BucketAdminSince
	}
	if sub.SubjectSince != nil && sub.SubjectSince.After(start) {
		start = *sub.SubjectSince
	}
	days := p.GraceDays
	if days < 0 {
		days = 0
	}
	d.GraceUntil = start.Add(time.Duration(days) * 24 * time.Hour)
	d.InGrace = now.Before(d.GraceUntil)
	if d.InGrace {
		if dangerous && d.HasLocalFactor {
			d.Code = CodeStepUp
		}
		return d
	}
	if !d.HasFactor || (dangerous && !d.HasLocalFactor) {
		d.Code = CodeMFARequired
		return d
	}
	if dangerous {
		d.Code = CodeStepUp
	}
	return d
}

func subject(p MFAPolicy, sub SubjectInput) bool {
	if sub.BucketAdmin {
		return true
	}
	if p.ApplyTenantAdmins && sub.TenantAdmin {
		return true
	}
	if p.ApplyGlobalAdmins && sub.Role == RoleAdministrator {
		return true
	}
	return false
}

// IdPFactorAccepted reports whether amr or acr from an ID token counts as 2FA.
// An empty accepted ACR list never matches. An empty accepted AMR list uses DefaultOIDCAMR.
func IdPFactorAccepted(amr []string, acr string, acceptedAMR, acceptedACR []string) bool {
	if len(acceptedAMR) == 0 {
		acceptedAMR = DefaultOIDCAMR
	}
	for _, got := range amr {
		got = strings.TrimSpace(got)
		if got == "" {
			continue
		}
		for _, want := range acceptedAMR {
			if strings.EqualFold(strings.TrimSpace(want), got) {
				return true
			}
		}
	}
	acr = strings.TrimSpace(acr)
	if acr == "" {
		return false
	}
	for _, want := range acceptedACR {
		if strings.TrimSpace(want) != "" && strings.EqualFold(want, acr) {
			return true
		}
	}
	return false
}

// AdminKeyAllowed is the S3 rule for a user access key. Grace does not exempt
// keys created before the policy: once the owner is a subject, admin calls
// require created_with_mfa. Ordinary object calls are not checked here.
func AdminKeyAllowed(policyEnabled, subject, createdWithMFA bool) bool {
	if !policyEnabled || !subject {
		return true
	}
	return createdWithMFA
}
