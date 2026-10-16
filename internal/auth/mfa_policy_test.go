package auth

import (
	"testing"
	"time"
)

func TestDecideMatrix(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	enabled := now.Add(-time.Hour)
	policy := MFAPolicy{
		Enabled: true, GraceDays: 0, EnabledAt: enabled,
		ApplyTenantAdmins: true, ApplyGlobalAdmins: true,
	}
	owner := SubjectInput{Role: RoleUser, BucketAdmin: true}
	noFactor := FactorState{}
	totp := FactorState{TOTP: true}
	idp := FactorState{IdPMFA: true}

	cases := []struct {
		name      string
		p         MFAPolicy
		sub       SubjectInput
		f         FactorState
		dangerous bool
		at        time.Time
		want      string
		grace     bool
	}{
		{"policy off", MFAPolicy{}, owner, noFactor, true, now, "", false},
		{"plain user", policy, SubjectInput{Role: RoleUser}, noFactor, true, now, "", false},
		{"owner grace 0 no factor", policy, owner, noFactor, true, now, CodeMFARequired, false},
		{"owner with totp steps up", policy, owner, totp, true, now, CodeStepUp, false},
		{"owner idp only needs local", policy, owner, idp, true, now, CodeMFARequired, false},
		{"grace allows no factor", MFAPolicy{Enabled: true, GraceDays: 7, EnabledAt: now, ApplyGlobalAdmins: true}, owner, noFactor, true, now.Add(time.Hour), "", true},
		{"grace still steps up with factor", MFAPolicy{Enabled: true, GraceDays: 7, EnabledAt: now}, owner, totp, true, now.Add(time.Hour), CodeStepUp, true},
		{"after grace 403", MFAPolicy{Enabled: true, GraceDays: 7, EnabledAt: now.Add(-8 * 24 * time.Hour)}, owner, noFactor, true, now, CodeMFARequired, false},
		{"tenant admin flag", policy, SubjectInput{Role: RoleUser, TenantAdmin: true}, noFactor, true, now, CodeMFARequired, false},
		{"tenant admin flag off", MFAPolicy{Enabled: true, GraceDays: 0, EnabledAt: enabled}, SubjectInput{Role: RoleUser, TenantAdmin: true}, noFactor, true, now, "", false},
		{"global admin flag", policy, SubjectInput{Role: RoleAdministrator}, noFactor, true, now, CodeMFARequired, false},
		{"global admin flag off", MFAPolicy{Enabled: true, GraceDays: 0, EnabledAt: enabled}, SubjectInput{Role: RoleAdministrator}, noFactor, true, now, "", false},
		{"later bucket admin restarts grace", MFAPolicy{Enabled: true, GraceDays: 7, EnabledAt: now.Add(-8 * 24 * time.Hour)}, SubjectInput{Role: RoleUser, BucketAdmin: true, BucketAdminSince: &now}, noFactor, true, now.Add(time.Hour), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.p, tc.sub, tc.f, tc.dangerous, tc.at)
			if got.Code != tc.want {
				t.Fatalf("code %q, want %q", got.Code, tc.want)
			}
			if got.InGrace != tc.grace {
				t.Fatalf("inGrace %v, want %v (until %s)", got.InGrace, tc.grace, got.GraceUntil)
			}
		})
	}
}

func TestIdPFactorAccepted(t *testing.T) {
	if !IdPFactorAccepted([]string{"mfa"}, "", nil, nil) {
		t.Fatal("default amr mfa")
	}
	if IdPFactorAccepted(nil, "urn:mfa", nil, nil) {
		t.Fatal("acr ignored when the list is empty")
	}
	if !IdPFactorAccepted(nil, "urn:mfa", nil, []string{"urn:mfa"}) {
		t.Fatal("configured acr")
	}
	if IdPFactorAccepted([]string{"pwd"}, "", nil, nil) {
		t.Fatal("pwd is not a factor")
	}
	if !IdPFactorAccepted([]string{"hwk"}, "", []string{"hwk"}, nil) {
		t.Fatal("custom amr")
	}
}

func TestAdminKeyAllowed(t *testing.T) {
	if !AdminKeyAllowed(false, true, false) {
		t.Fatal("policy off")
	}
	if !AdminKeyAllowed(true, false, false) {
		t.Fatal("not a subject")
	}
	if AdminKeyAllowed(true, true, false) {
		t.Fatal("subject key without flag")
	}
	if !AdminKeyAllowed(true, true, true) {
		t.Fatal("flag set")
	}
}
