package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/DirektorBani/datasafe/internal/auth"
	wa "github.com/DirektorBani/datasafe/internal/auth/webauthn"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/go-webauthn/webauthn/webauthn"
)

type webauthnSessionStore struct {
	mu       sync.Mutex
	sessions map[string]webauthn.SessionData
}

func newWebAuthnSessionStore() *webauthnSessionStore {
	return &webauthnSessionStore{sessions: map[string]webauthn.SessionData{}}
}

func (st *webauthnSessionStore) put(key string, data webauthn.SessionData) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[key] = data
}

func (st *webauthnSessionStore) pop(key string) (webauthn.SessionData, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	data, ok := st.sessions[key]
	delete(st.sessions, key)
	return data, ok
}

func (s *Server) webauthnService(r *http.Request) (*webauthn.WebAuthn, error) {
	rpID, origin := wa.RPFromRequest(r)
	return wa.NewService(rpID, origin)
}

func (s *Server) webauthnUser(rec metadata.UserRecord) wa.User {
	return wa.User{
		ID:          rec.ID,
		Username:    rec.Username,
		DisplayName: rec.Username,
		Credentials: wa.ParseCredentials(rec.WebAuthnCredentials),
	}
}

func (s *Server) handleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	waSvc, err := s.webauthnService(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	wUser := s.webauthnUser(user)
	options, session, err := waSvc.BeginRegistration(wUser)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.webauthnSessions.put("reg:"+user.ID, *session)
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) handleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	session, ok := s.webauthnSessions.pop("reg:" + user.ID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "registration session expired"})
		return
	}
	waSvc, err := s.webauthnService(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	wUser := s.webauthnUser(user)
	cred, err := wa.FinishRegistration(waSvc, wUser, session, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	creds := append(wUser.Credentials, cred)
	encoded, err := wa.MarshalCredentials(creds)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	user.WebAuthnCredentials = encoded
	user.MFAEnabled = true
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "passkeys": len(creds)})
}

func (s *Server) handleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string `json:"mfa_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MFAToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mfa_token required"})
		return
	}
	userID, err := s.jwt.ValidateMFAToken(req.MFAToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid mfa token"})
		return
	}
	user, err := s.meta.GetUser(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	if user.WebAuthnCredentials == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "no passkeys enrolled"})
		return
	}
	waSvc, err := s.webauthnService(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	options, session, err := wa.BeginLogin(waSvc, s.webauthnUser(user))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.webauthnSessions.put("login:"+user.ID, *session)
	writeJSON(w, http.StatusOK, options)
}

func (s *Server) handleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string `json:"mfa_token"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	userID := ""
	if req.MFAToken != "" {
		userID, _ = s.jwt.ValidateMFAToken(req.MFAToken)
	}
	if userID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mfa_token required"})
		return
	}
	user, err := s.meta.GetUser(userID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	session, ok := s.webauthnSessions.pop("login:" + user.ID)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "login session expired"})
		return
	}
	waSvc, err := s.webauthnService(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if _, err := wa.FinishLogin(waSvc, s.webauthnUser(user), session, r); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	token, err := s.jwt.Issue(auth.TokenInfo{
		Username: user.Username,
		UserID:   user.ID,
		Role:     user.Role,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "token issue failed"})
		return
	}
	s.logActivityAs(user.Username, clientIP(r), metadata.ActionLogin, "session", user.Username)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":      token,
		"expires_in": 86400,
		"username":   user.Username,
		"role":       user.Role,
		"user_id":    user.ID,
	})
}
