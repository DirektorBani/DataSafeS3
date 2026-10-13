package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	wa "github.com/DirektorBani/datasafe/internal/auth/webauthn"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

type stepUpRequest struct {
	Action   string `json:"action"`
	Target   string `json:"target"`
	TOTPCode string `json:"totp_code"`
	Code     string `json:"code"`
}

func (s *Server) handleMFAPolicyGet(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"policy":            cfg.MFA,
		"dangerous_actions": auth.DangerousActions,
		"users_without_mfa": s.usersMissingMFA(cfg),
	})
}

func (s *Server) handleMFAPolicyPut(w http.ResponseWriter, r *http.Request) {
	var next metadata.MFASettings
	if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if next.GracePeriodDays != nil {
		d := *next.GracePeriodDays
		if d < 0 || d > 90 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "grace_period_days must be between 0 and 90"})
			return
		}
	} else {
		d := 7
		next.GracePeriodDays = &d
	}
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	prev := cfg.MFA
	cfg.MFA = metadata.NormalizeMFA(next, prev)
	if err := s.meta.PutSystemConfig(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	info, _ := authFrom(r)
	s.publishMFAEvent(r, "MfaPolicyChanged", map[string]any{
		"user": info.Username,
		"before": map[string]any{
			"require_bucket_admin_mfa": prev.RequireBucketAdminMFA,
			"grace_period_days":        prev.GraceDays(),
			"apply_to_tenant_admins":   prev.ApplyToTenantAdmins,
			"apply_to_global_admins":   prev.AppliesToGlobalAdmins(),
		},
		"after": map[string]any{
			"require_bucket_admin_mfa": cfg.MFA.RequireBucketAdminMFA,
			"grace_period_days":        cfg.MFA.GraceDays(),
			"apply_to_tenant_admins":   cfg.MFA.ApplyToTenantAdmins,
			"apply_to_global_admins":   cfg.MFA.AppliesToGlobalAdmins(),
		},
	})
	s.logActivity(r, metadata.ActionMFAPolicyChanged, "mfa", "policy")
	writeJSON(w, http.StatusOK, map[string]any{"policy": cfg.MFA})
}

type mfaUserView struct {
	UserID     string    `json:"user_id"`
	Username   string    `json:"username"`
	GraceUntil time.Time `json:"grace_until"`
	InGrace    bool      `json:"in_grace"`
}

func (s *Server) usersMissingMFA(cfg metadata.SystemConfig) []mfaUserView {
	users, err := s.meta.ListUsers()
	if err != nil {
		return []mfaUserView{}
	}
	var out []mfaUserView
	for _, user := range users {
		d := auth.Decide(s.mfaPolicy(cfg), s.subjectInput(user), s.factorState(user), false, time.Now().UTC())
		if !d.Subject || d.HasFactor {
			continue
		}
		out = append(out, mfaUserView{
			UserID: user.ID, Username: user.Username, GraceUntil: d.GraceUntil, InGrace: d.InGrace,
		})
	}
	if out == nil {
		out = []mfaUserView{}
	}
	return out
}

func (s *Server) handleStepUp(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req stepUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	code := req.TOTPCode
	if code == "" {
		code = req.Code
	}
	s.finishStepUp(w, r, info.UserID, info.SessionID, req.Action, req.Target, func(user *metadata.UserRecord) bool {
		return s.validateUserMFACode(user, code)
	})
}

func (s *Server) handleStepUpWebAuthnBegin(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req stepUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if !auth.IsDangerousAction(req.Action) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown action"})
		return
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil || !userHasWebAuthn(user) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "webauthn not enrolled"})
		return
	}
	waSvc, err := s.webauthnService(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	options, session, err := wa.BeginLogin(waSvc, s.webauthnUser(user))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.webauthnSessions.put("stepup:"+user.ID, *session)
	s.webauthnSessions.putPending(user.ID, req.Action, req.Target)
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) handleStepUpWebAuthnFinish(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	action, target, ok := s.webauthnSessions.takePending(info.UserID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "step-up session expired"})
		return
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	session, ok := s.webauthnSessions.pop("stepup:" + user.ID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "step-up session expired"})
		return
	}
	waSvc, err := s.webauthnService(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.finishStepUp(w, r, info.UserID, info.SessionID, action, target, func(u *metadata.UserRecord) bool {
		_, err := wa.FinishLogin(waSvc, s.webauthnUser(*u), session, r)
		return err == nil
	})
}

func (s *Server) finishStepUp(w http.ResponseWriter, r *http.Request, userID, sessionID, action, target string, prove func(*metadata.UserRecord) bool) {
	if !auth.IsDangerousAction(action) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "unknown action"})
		return
	}
	user, err := s.meta.GetUser(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	if !s.factorState(user).HasLocalFactor() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "mfa_required", "message": "требуется 2FA"})
		return
	}
	now := time.Now().UTC()
	locked, err := s.meta.MFAStepUpLocked(user.ID, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if locked.Locked {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": "mfa_step_up_locked", "retry_after_sec": int(time.Until(locked.LockedUntil).Seconds()),
		})
		return
	}
	if !prove(&user) {
		state, ferr := s.meta.NoteMFAStepUpFailure(user.ID, now)
		if ferr == nil && state.Locked {
			s.logActivity(r, metadata.ActionMFAStepUpLocked, "mfa", user.Username)
		}
		_ = s.meta.UpdateUser(user)
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid code"})
		return
	}
	_ = s.meta.ClearMFAStepUpFailures(user.ID)
	_ = s.meta.UpdateUser(user)
	token, exp, err := s.issueActionToken(user.ID, sessionID, action, target)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token, "expires_at": exp.Format(time.RFC3339), "action": action, "target": target,
	})
}

func (s *Server) issueActionToken(userID, sessionID, action, target string) (string, time.Time, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	token := hex.EncodeToString(buf)
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)
	exp := time.Now().UTC().Add(mfaActionTokenTTL())
	err := s.meta.PutMFAActionToken(metadata.MFAActionToken{
		Hash:      hashMFAToken(token),
		UserID:    userID,
		SessionID: sessionID,
		Action:    action,
		Target:    target,
		Nonce:     hex.EncodeToString(nonce),
		ExpiresAt: exp,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, exp, nil
}

func hashMFAToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) handleMFAReset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if stringsTrim(req.Reason) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "reason required"})
		return
	}
	if !s.allowDangerous(w, r, auth.ActionMFAReset, id) {
		return
	}
	user, err := s.meta.GetUser(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	user.MFAEnabled = false
	user.TOTPSecret = ""
	user.RecoveryCodes = nil
	user.WebAuthnCredentials = ""
	user.IdPMFA = false
	user.MFAResetRequired = true
	user.TOTPLastStep = 0
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	_ = s.meta.ResetTOTPStep(user.ID)
	info, _ := authFrom(r)
	s.logActivity(r, metadata.ActionMFAReset, "user", user.ID+" reason:"+req.Reason)
	s.publishMFAEvent(r, "MfaReset", map[string]any{
		"actor": info.Username, "user": user.Username, "user_id": user.ID, "reason": req.Reason,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func stringsTrim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func (st *webauthnSessionStore) putPending(userID, action, target string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.pending == nil {
		st.pending = map[string]stepUpPending{}
	}
	st.pending[userID] = stepUpPending{Action: action, Target: target}
}

func (st *webauthnSessionStore) takePending(userID string) (string, string, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	p, ok := st.pending[userID]
	delete(st.pending, userID)
	if !ok {
		return "", "", false
	}
	return p.Action, p.Target, true
}

type stepUpPending struct {
	Action string
	Target string
}
