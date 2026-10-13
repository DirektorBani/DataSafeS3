package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	wa "github.com/DirektorBani/datasafe/internal/auth/webauthn"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

const mfaTokenHeader = "X-DataSafe-MFA-Token"

const adminKeyWithoutMFAMessage = "ключ создан без 2FA; создайте новый ключ после входа с 2FA"

func mfaActionTokenTTL() time.Duration {
	raw := strings.TrimSpace(os.Getenv("STORAGE_MFA_ACTION_TOKEN_TTL"))
	if raw == "" {
		return 60 * time.Second
	}
	sec, err := strconv.Atoi(raw)
	if err != nil || sec < 30 || sec > 300 {
		return 60 * time.Second
	}
	return time.Duration(sec) * time.Second
}

func (s *Server) mfaPolicy(cfg metadata.SystemConfig) auth.MFAPolicy {
	m := cfg.MFA
	enabledAt := time.Now().UTC()
	if m.EnabledAt != nil {
		enabledAt = m.EnabledAt.UTC()
	}
	return auth.MFAPolicy{
		Enabled:           m.RequireBucketAdminMFA,
		GraceDays:         m.GraceDays(),
		ApplyTenantAdmins: m.ApplyToTenantAdmins,
		ApplyGlobalAdmins: m.AppliesToGlobalAdmins(),
		EnabledAt:         enabledAt,
	}
}

func (s *Server) factorState(user metadata.UserRecord) auth.FactorState {
	return auth.FactorState{
		TOTP:     user.MFAEnabled,
		WebAuthn: len(wa.ParseCredentials(user.WebAuthnCredentials)) > 0,
		IdPMFA:   user.IdPMFA,
	}
}

func (s *Server) subjectInput(user metadata.UserRecord) auth.SubjectInput {
	facts := metadata.MFASubjectFor(s.meta, user)
	return auth.SubjectInput{
		Role:             facts.Role,
		BucketAdmin:      facts.BucketAdmin,
		TenantAdmin:      facts.TenantAdmin,
		BucketAdminSince: facts.BucketAdminSince,
		SubjectSince:     facts.SubjectSince,
	}
}

func (s *Server) mfaDecision(user metadata.UserRecord, dangerous bool) auth.Decision {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		return auth.Decision{}
	}
	return auth.Decide(s.mfaPolicy(cfg), s.subjectInput(user), s.factorState(user), dangerous, time.Now().UTC())
}

func mfaChannel(r *http.Request) string {
	if strings.EqualFold(r.Header.Get("X-DataSafe-Channel"), "console") {
		return "console"
	}
	return "API"
}

// allowDangerous enforces the bucket-admin MFA policy for one dangerous action.
// It returns true when the handler should continue.
func (s *Server) allowDangerous(w http.ResponseWriter, r *http.Request, action, target string) bool {
	if !auth.IsDangerousAction(action) {
		return true
	}
	info, ok := authFrom(r)
	if !ok || info.UserID == "" {
		return true
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		return true
	}
	d := s.mfaDecision(user, true)
	if !d.Subject || d.Code == "" {
		return true
	}
	channel := mfaChannel(r)
	if d.Code == auth.CodeMFARequired {
		s.noteMFARequiredDenied(r, user, action, target, channel)
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":   "mfa_required",
			"message": "требуется 2FA",
			"action":  action,
		})
		return false
	}
	return s.consumePresentedToken(w, r, user, action, target, channel)
}

func (s *Server) consumePresentedToken(w http.ResponseWriter, r *http.Request, user metadata.UserRecord, action, target, channel string) bool {
	raw := strings.TrimSpace(r.Header.Get(mfaTokenHeader))
	if raw == "" {
		s.publishMFAEvent(r, "MfaStepUpRequired", map[string]any{
			"user": user.Username, "user_id": user.ID, "action": action, "target": target, "channel": channel,
		})
		s.logActivity(r, metadata.ActionMFAStepUpRequired, "mfa", action+":"+target)
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "step_up_required", "action": action, "target": target,
		})
		return false
	}
	info, _ := authFrom(r)
	if _, err := s.meta.ConsumeMFAActionToken(hashMFAToken(raw), metadata.MFATokenMatch{
		UserID: info.UserID, SessionID: info.SessionID, Action: action, Target: target,
	}, time.Now().UTC()); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "mfa_token_invalid"})
		return false
	}
	s.publishMFAEvent(r, "MfaStepUpVerified", map[string]any{
		"user": user.Username, "user_id": user.ID, "action": action, "target": target, "channel": channel,
	})
	s.logActivity(r, metadata.ActionMFAStepUpVerified, "mfa", action+":"+target)
	return true
}

func (s *Server) noteMFARequiredDenied(r *http.Request, user metadata.UserRecord, action, target, channel string) {
	ok, err := s.meta.ClaimMFADeniedSlot(user.ID, action, time.Now().UTC())
	if err != nil || !ok {
		return
	}
	s.publishMFAEvent(r, "MfaRequiredDenied", map[string]any{
		"user": user.Username, "user_id": user.ID, "action": action, "target": target,
		"bucket": target, "channel": channel,
	})
	s.logActivity(r, metadata.ActionMFARequiredDenied, "mfa", action+":"+target)
}

func (s *Server) publishMFAEvent(r *http.Request, eventType string, payload map[string]any) {
	s.publishEvent(r.Context(), eventType, "", "", "", "", payload)
}

func (s *Server) noteBucketAdmin(userID string) {
	if userID == "" {
		return
	}
	user, err := s.meta.GetUser(userID)
	if err != nil || user.BucketAdminSince != nil {
		return
	}
	now := time.Now().UTC()
	user.BucketAdminSince = &now
	_ = s.meta.UpdateUser(user)
}

func (s *Server) noteTenantAdmin(userID string) {
	if userID == "" {
		return
	}
	user, err := s.meta.GetUser(userID)
	if err != nil || user.MFAPolicySince != nil {
		return
	}
	now := time.Now().UTC()
	user.MFAPolicySince = &now
	_ = s.meta.UpdateUser(user)
}

func (s *Server) runMFATokenPurge(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = s.meta.PurgeMFAActionTokens(time.Now().UTC())
		}
	}
}

func (s *Server) accessKeyMFAFlag(userID string) bool {
	user, err := s.meta.GetUser(userID)
	if err != nil {
		return false
	}
	d := s.mfaDecision(user, true)
	return d.Subject && d.HasLocalFactor
}

func userHasWebAuthn(user metadata.UserRecord) bool {
	return len(wa.ParseCredentials(user.WebAuthnCredentials)) > 0
}
