package s3

import (
	"net/http"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	wa "github.com/DirektorBani/datasafe/internal/auth/webauthn"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

// adminS3WithoutMFAMessage is the AccessDenied text for a key created before MFA.
const adminS3WithoutMFAMessage = "ключ создан без 2FA; создайте новый ключ после входа с 2FA"

// denyAdminS3WithoutMFA rejects administrative S3 calls made with a user access
// key that was not created from an MFA-confirmed session. The bootstrap root
// key has no owner and is unchanged. Object operations do not call this.
// S3 does not accept X-DataSafe-MFA-Token; per-action confirmation is console/API only.
func (h *Handler) denyAdminS3WithoutMFA(w http.ResponseWriter, r *http.Request, accessKey string) bool {
	if h.Svc.adminKeyAllowed(accessKey) {
		return false
	}
	writeError(w, r, http.StatusForbidden, "AccessDenied", adminS3WithoutMFAMessage)
	return true
}

func (s *Service) adminKeyAllowed(accessKey string) bool {
	rec, err := s.Meta.GetAccessKey(accessKey)
	if err != nil || rec.OwnerID == "" {
		return true
	}
	user, err := s.Meta.GetUser(rec.OwnerID)
	if err != nil {
		return true
	}
	cfg, err := s.Meta.GetSystemConfig()
	if err != nil || !cfg.MFA.RequireBucketAdminMFA {
		return true
	}
	facts := metadata.MFASubjectFor(s.Meta, user)
	enabledAt := time.Now().UTC()
	if cfg.MFA.EnabledAt != nil {
		enabledAt = cfg.MFA.EnabledAt.UTC()
	}
	d := auth.Decide(auth.MFAPolicy{
		Enabled:           true,
		GraceDays:         cfg.MFA.GraceDays(),
		ApplyTenantAdmins: cfg.MFA.ApplyToTenantAdmins,
		ApplyGlobalAdmins: cfg.MFA.AppliesToGlobalAdmins(),
		EnabledAt:         enabledAt,
	}, auth.SubjectInput{
		Role:             facts.Role,
		BucketAdmin:      facts.BucketAdmin,
		TenantAdmin:      facts.TenantAdmin,
		BucketAdminSince: facts.BucketAdminSince,
		SubjectSince:     facts.SubjectSince,
	}, auth.FactorState{
		TOTP:     user.MFAEnabled,
		WebAuthn: len(wa.ParseCredentials(user.WebAuthnCredentials)) > 0,
		IdPMFA:   user.IdPMFA,
	}, false, time.Now().UTC())
	return auth.AdminKeyAllowed(true, d.Subject, rec.CreatedWithMFA)
}

func retentionShortens(existing *time.Time, until time.Time) bool {
	return existing != nil && until.Before(*existing)
}

func complianceOrShorten(mode string, existing *time.Time, until time.Time) bool {
	if strings.EqualFold(mode, "COMPLIANCE") {
		return true
	}
	return retentionShortens(existing, until)
}
