package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/federation"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"golang.org/x/oauth2"
)

func (s *Server) ldapSettingsFrom(cfg metadata.SystemConfig) auth.LDAPSettings {
	ldap := cfg.LDAP
	return auth.LDAPSettings{
		URL:          auth.ResolveLDAPURL(ldap.URL),
		BindDN:       ldap.BindDN,
		BindPassword: ldap.BindPassword,
		BaseDN:       ldap.BaseDN,
		GroupDN:      ldap.GroupDN,
		UserAttr:     ldap.UserAttr,
		GroupAttr:    ldap.GroupAttr,
	}
}

func (s *Server) handleLDAPTest(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	var req metadata.LDAPConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.URL != "" {
		cfg.LDAP = req
	}
	if cfg.LDAP.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ldap url required"})
		return
	}
	if err := validateLDAPTLS(cfg.LDAP.URL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := auth.TestLDAPConn(s.ldapSettingsFrom(cfg)); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": "connection successful"})
}

func (s *Server) handleLDAPSync(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.LDAP.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "ldap not enabled"})
		return
	}
	cfg = s.mergeEnvConfig(cfg)
	res, err := s.performLDAPSync(cfg)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "ldap", fmt.Sprintf("synced %d users (created=%d updated=%d suspended=%d)", res.synced(), res.Created, res.Updated, res.Suspended))
	writeJSON(w, http.StatusOK, map[string]any{
		"synced":      res.synced(),
		"created":     res.Created,
		"updated":     res.Updated,
		"suspended":   res.Suspended,
		"total_found": res.TotalFound,
	})
}

func (s *Server) tryLDAPLogin(username, password string) (metadata.UserRecord, error) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.LDAP.Enabled {
		return metadata.UserRecord{}, auth.ErrInvalidCredentials
	}
	lu, err := auth.AuthenticateLDAP(s.ldapSettingsFrom(cfg), username, password)
	if err != nil {
		return metadata.UserRecord{}, err
	}
	user, err := s.meta.GetUserByUsername(lu.Username)
	if err != nil {
		if !cfg.LDAP.SyncOnLogin {
			return metadata.UserRecord{}, auth.ErrInvalidCredentials
		}
		hash, _ := auth.HashPassword(randomHex(8))
		role := auth.MapLDAPRole(lu.Groups, cfg.LDAP.GroupRoleMap, metadata.RoleUser)
		user = metadata.UserRecord{
			ID:           randomID(),
			Username:     lu.Username,
			Email:        lu.Email,
			PasswordHash: hash,
			Role:         role,
			Status:       metadata.StatusActive,
			TenantID:     metadata.DefaultTenantID,
			AuthSource:   "ldap",
			CreatedAt:    time.Now().UTC(),
		}
		if err := s.meta.PutUser(user); err != nil {
			return metadata.UserRecord{}, err
		}
	}
	s.syncUserTenantGroupsFromExternal(user.ID, lu.Groups)
	return user, nil
}

func (s *Server) handleOIDCPublicConfig(w http.ResponseWriter, r *http.Request) {
	cfg, _ := s.meta.GetSystemConfig()
	resp := map[string]any{
		"enabled": cfg.OIDC.Enabled,
		"issuer":  cfg.OIDC.Issuer,
	}
	if cfg.OIDC.Enabled {
		issuers := s.oidcIssuers(cfg)
		issuerURL := issuers.Public
		if issuerURL == "" {
			issuerURL = issuers.Internal
		}
		if _, _, _, err := auth.DiscoverOIDCEndpoints(issuerURL, http.DefaultClient); err != nil {
			resp["issuer_reachable"] = false
			resp["issuer_error"] = err.Error()
		} else {
			resp["issuer_reachable"] = true
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) oidcIssuers(cfg metadata.SystemConfig) auth.OIDCIssuers {
	return auth.ResolveOIDCIssuers(cfg.OIDC.Issuer, cfg.OIDC.InternalIssuer)
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.OIDC.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "oidc not enabled"})
		return
	}
	state := randomHex(16)
	issuers := s.oidcIssuers(cfg)
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		RedirectURL:  cfg.OIDC.RedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     auth.BrowserOAuthEndpoint(issuers, http.DefaultClient),
	}
	url := oauthCfg.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.OIDC.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "oidc not enabled"})
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing code"})
		return
	}
	issuers := s.oidcIssuers(cfg)
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		RedirectURL:  cfg.OIDC.RedirectURL,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     auth.ServerOAuthEndpoint(issuers, http.DefaultClient),
	}
	token, err := oauthCfg.Exchange(r.Context(), code)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token exchange failed: " + err.Error()})
		return
	}
	jwtToken, err := s.completeOIDCLogin(cfg, token)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	exchangeCode, err := s.oidcExchange.Issue(jwtToken)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "exchange code failed"})
		return
	}
	redirect := "/login?exchange_code=" + url.QueryEscape(exchangeCode) + "&auth_source=oidc"
	http.Redirect(w, r, redirect, http.StatusFound)
}

func (s *Server) handleOIDCExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExchangeCode string `json:"exchange_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.ExchangeCode) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "exchange_code required"})
		return
	}
	jwtToken, ok := s.oidcExchange.Redeem(req.ExchangeCode)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired exchange code"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": jwtToken, "auth_source": "oidc"})
}

// handleOIDCPasswordLogin exchanges username/password via OIDC ROPC (Keycloak direct access grants).
// Intended for automated tests; production clients should use the authorization code flow.
func (s *Server) handleOIDCPasswordLogin(w http.ResponseWriter, r *http.Request) {
	if !oidcROPCEnabled() {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "oidc resource-owner password grant disabled"})
		return
	}
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.OIDC.Enabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "oidc not enabled"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username and password required"})
		return
	}
	issuers := s.oidcIssuers(cfg)
	oauthCfg := &oauth2.Config{
		ClientID:     cfg.OIDC.ClientID,
		ClientSecret: cfg.OIDC.ClientSecret,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     auth.ServerOAuthEndpoint(issuers, http.DefaultClient),
	}
	token, err := oauthCfg.PasswordCredentialsToken(r.Context(), req.Username, req.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "oidc password grant failed: " + err.Error()})
		return
	}
	jwtToken, err := s.completeOIDCLogin(cfg, token)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": jwtToken, "auth_source": "oidc"})
}

func (s *Server) completeOIDCLogin(cfg metadata.SystemConfig, token *oauth2.Token) (string, error) {
	issuers := s.oidcIssuers(cfg)
	userInfoURL := auth.ServerUserInfoURL(issuers, http.DefaultClient)
	username, email, err := auth.ParseOIDCUserFromToken(token, userInfoURL, http.DefaultClient)
	if err != nil {
		return "", err
	}
	groupsClaim := cfg.OIDC.GroupsClaim
	if groupsClaim == "" {
		groupsClaim = "groups"
	}
	oidcGroups := auth.ParseOIDCGroupsFromToken(token, groupsClaim, userInfoURL, http.DefaultClient)
	user, err := s.meta.GetUserByUsername(username)
	if err != nil {
		hash, _ := auth.HashPassword(randomHex(8))
		user = metadata.UserRecord{
			ID:           randomID(),
			Username:     username,
			Email:        email,
			PasswordHash: hash,
			Role:         metadata.RoleUser,
			Status:       metadata.StatusActive,
			TenantID:     metadata.DefaultTenantID,
			AuthSource:   "oidc",
			CreatedAt:    time.Now().UTC(),
		}
		if err := s.meta.PutUser(user); err != nil {
			return "", err
		}
	}
	s.syncUserTenantGroupsFromExternal(user.ID, oidcGroups)
	now := time.Now().UTC()
	user.LastLogin = &now
	_ = s.meta.UpdateUser(user)
	s.ensureHomeBucket(auth.TokenInfo{
		Username: user.Username,
		UserID:   user.ID,
		Role:     user.Role,
	})
	sessionID := randomHex(16)
	jwtToken, err := s.jwt.IssueWithTTL(auth.TokenInfo{
		Username:   user.Username,
		UserID:     user.ID,
		Role:       user.Role,
		AuthSource: auth.AuthSourceOIDC,
		SessionID:  sessionID,
	}, auth.OIDCSessionJWTTTL)
	if err != nil {
		return "", errors.New("token issue failed")
	}
	idToken, _ := token.Extra("id_token").(string)
	s.oidcSessions.Put(sessionID, auth.OIDCSession{
		AccessToken: token.AccessToken,
		IDToken:     idToken,
		Username:    user.Username,
		CreatedAt:   time.Now().UTC(),
	})
	return jwtToken, nil
}

func (s *Server) validateOIDCSession(ctx context.Context, sessionID string) (bool, error) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil || !cfg.OIDC.Enabled {
		return false, nil
	}
	issuers := s.oidcIssuers(cfg)
	introspect := func(ctx context.Context, accessToken string) (bool, error) {
		return auth.OIDCIntrospect(ctx, issuers, cfg.OIDC.ClientID, cfg.OIDC.ClientSecret, accessToken, http.DefaultClient)
	}
	return s.oidcSessions.Active(ctx, sessionID, introspect)
}

func (s *Server) handleMFAEnroll(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	if user.MFAEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mfa already enabled"})
		return
	}
	secret, otpauthURI, qrPNG, err := auth.GenerateTOTPEnrollment(user.Username)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	enc, err := auth.EncryptTOTPSecret(s.mfaEncryptionKey(), secret)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	user.TOTPSecret = enc
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":      secret,
		"otpauth_uri": otpauthURI,
		"qr_url":      otpauthURI,
		"qr_code":     qrPNG,
	})
}

func (s *Server) handleMFAVerifyEnroll(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	if user.MFAEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mfa already enabled"})
		return
	}
	secret, err := s.userTOTPSecret(user)
	if err != nil || secret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "enroll first"})
		return
	}
	if !auth.ValidateTOTP(secret, req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid code"})
		return
	}
	plainCodes, err := auth.GenerateRecoveryCodes(10)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	hashed := make([]string, len(plainCodes))
	for i, c := range plainCodes {
		h, err := auth.HashRecoveryCode(c)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		hashed[i] = h
	}
	user.MFAEnabled = true
	user.RecoveryCodes = hashed
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "mfa", "enabled")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "recovery_codes": plainCodes})
}

func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	var req struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	user, err := s.meta.GetUser(info.UserID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "user not found"})
		return
	}
	if !user.MFAEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mfa not enabled"})
		return
	}
	if req.Password == "" || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid password"})
		return
	}
	if !s.validateUserMFACode(&user, req.Code) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid code"})
		return
	}
	user.MFAEnabled = false
	user.TOTPSecret = ""
	user.RecoveryCodes = nil
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionSettingsChanged, "mfa", "disabled")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleMFALogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
		TOTPCode string `json:"totp_code"`
		MFACode  string `json:"mfa_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	code := req.Code
	if code == "" {
		code = req.TOTPCode
	}
	if code == "" {
		code = req.MFACode
	}
	userID, err := s.jwt.ValidateMFAToken(req.MFAToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid or expired mfa token"})
		return
	}
	user, err := s.meta.GetUser(userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid credentials"})
		return
	}
	if user.Status != metadata.StatusActive {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "account suspended"})
		return
	}
	if !user.MFAEnabled {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "mfa not enabled"})
		return
	}
	if !s.validateUserMFACode(&user, code) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid mfa code"})
		return
	}
	now := time.Now().UTC()
	user.LastLogin = &now
	if err := s.meta.UpdateUser(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.markAdminFirstLoginIfNeeded(user)
	tokenInfo := auth.TokenInfo{
		Username: user.Username,
		UserID:   user.ID,
		Role:     user.Role,
	}
	s.ensureHomeBucket(tokenInfo)
	token, err := s.jwt.Issue(tokenInfo)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "token issue failed"})
		return
	}
	s.logActivityAs(user.Username, clientIP(r), metadata.ActionLogin, "session", user.Username)
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token, "expires_in": 86400,
		"username": user.Username, "role": user.Role, "user_id": user.ID,
	})
}

func (s *Server) checkObjectDeletable(bucket, key, versionID string) error {
	rec, err := s.meta.GetObjectVersion(bucket, key, versionID)
	if err != nil {
		return err
	}
	if rec.LegalHold {
		return metadata.ErrLegalHold
	}
	if rec.RetentionUntil != nil && time.Now().UTC().Before(*rec.RetentionUntil) {
		return metadata.ErrRetentionLocked
	}
	bucketRec, err := s.meta.GetBucket(bucket)
	if err == nil && bucketRec.ObjectLock && bucketRec.RetentionDays > 0 {
		retentionEnd := rec.CreatedAt
		if retentionEnd.IsZero() {
			retentionEnd = rec.LastModified
		}
		until := retentionEnd.Add(time.Duration(bucketRec.RetentionDays) * 24 * time.Hour)
		if time.Now().UTC().Before(until) {
			return metadata.ErrRetentionLocked
		}
	}
	return nil
}

func (s *Server) handleSetLegalHold(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	info, _ := authFrom(r)
	if !s.canAccessBucket(info, bucket) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	var req struct {
		Key       string `json:"key"`
		VersionID string `json:"version_id"`
		Hold      bool   `json:"hold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if err := s.meta.SetObjectLegalHold(bucket, req.Key, req.VersionID, req.Hold); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	s.logActivity(r, metadata.ActionBucketUpdated, "object", bucket+"/"+req.Key)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "legal_hold": req.Hold})
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	info, _ := authFrom(r)
	memberships := s.userTenantMemberships(info.UserID)
	if !auth.CanManageAnyTenant(info.Role, memberships) {
		writeJSON(w, http.StatusForbidden, map[string]any{"error": "forbidden"})
		return
	}
	tenants, err := s.meta.ListTenants()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !auth.IsAdmin(info.Role) {
		adminTenants := map[string]struct{}{}
		for _, m := range s.userTenantMemberships(info.UserID) {
			if m.Role == auth.TenantRoleAdmin {
				adminTenants[m.TenantID] = struct{}{}
			}
		}
		filtered := make([]metadata.TenantRecord, 0)
		for _, t := range tenants {
			if _, ok := adminTenants[t.ID]; ok {
				filtered = append(filtered, t)
			}
		}
		tenants = filtered
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
}

func (s *Server) handleCreateTenant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required"})
		return
	}
	rec := metadata.TenantRecord{
		ID:        randomID(),
		Name:      req.Name,
		Status:    metadata.StatusActive,
		CreatedAt: time.Now().UTC(),
	}
	if err := s.meta.PutTenant(rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"tenant": rec})
}

func (s *Server) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteTenant(id); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListGatewayConnections(w http.ResponseWriter, r *http.Request) {
	conns, err := s.meta.ListGatewayConnections()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	type safe struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Endpoint  string    `json:"endpoint"`
		Region    string    `json:"region"`
		AccessKey string    `json:"access_key"`
		PathStyle bool      `json:"path_style"`
		TLSVerify bool      `json:"tls_verify"`
		Status    string    `json:"status"`
		LastCheck time.Time `json:"last_check"`
		CreatedAt time.Time `json:"created_at"`
	}
	var out []safe
	for _, c := range conns {
		out = append(out, safe{
			ID: c.ID, Name: c.Name, Endpoint: c.Endpoint, Region: c.Region,
			AccessKey: c.AccessKey, PathStyle: c.PathStyle, TLSVerify: c.TLSVerify,
			Status: c.Status, LastCheck: c.LastCheck, CreatedAt: c.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": out})
}

func (s *Server) handleCreateGatewayConnection(w http.ResponseWriter, r *http.Request) {
	var req metadata.GatewayConnection
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name == "" || req.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and endpoint required"})
		return
	}
	req.ID = randomID()
	req.CreatedAt = time.Now().UTC()
	req.Status = "unknown"
	if err := s.meta.PutGatewayConnection(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"connection": req})
}

func (s *Server) handleDeleteGatewayConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rules, err := s.meta.ListReplicationRules()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for _, rule := range rules {
		if rule.DestConnection == id {
			writeJSON(w, http.StatusConflict, map[string]any{
				"error": fmt.Sprintf("connection in use by replication rule %s → %s", rule.SourceBucket, rule.DestBucket),
			})
			return
		}
	}
	if err := s.meta.DeleteGatewayConnection(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTestGatewayConnection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	conn, err := s.meta.GetGatewayConnection(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	ok, msg := s.testS3Connection(conn)
	conn.LastCheck = time.Now().UTC()
	conn.Status = "ok"
	if !ok {
		conn.Status = "error"
	}
	_ = s.meta.PutGatewayConnection(conn)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok, "message": msg, "status": conn.Status})
}

func (s *Server) testS3Connection(conn metadata.GatewayConnection) (bool, string) {
	client, err := s.gatewayS3Client(conn)
	if err != nil {
		return false, err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return false, err.Error()
	}
	return true, "connected"
}

func (s *Server) handleListReplicationRules(w http.ResponseWriter, r *http.Request) {
	rules, err := s.meta.ListReplicationRules()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (s *Server) handleCreateReplicationRule(w http.ResponseWriter, r *http.Request) {
	var req metadata.ReplicationRule
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.SourceBucket == "" || req.DestConnection == "" || req.DestBucket == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "source_bucket, dest_connection_id, dest_bucket required"})
		return
	}
	if _, err := s.meta.GetGatewayConnection(req.DestConnection); err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "dest connection not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	req.ID = randomID()
	req.CreatedAt = time.Now().UTC()
	req.Enabled = true
	if err := s.meta.PutReplicationRule(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	go s.enqueueFullBucketScan(req)
	s.logActivity(r, metadata.ActionSettingsChanged, "gateway", "replication rule "+req.SourceBucket+"→"+req.DestBucket)
	writeJSON(w, http.StatusCreated, map[string]any{"rule": req})
}

func (s *Server) handleDeleteReplicationRule(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteReplicationRule(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTriggerSyncJob(w http.ResponseWriter, r *http.Request) {
	ruleID := r.PathValue("id")
	rule, err := s.meta.GetReplicationRule(ruleID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	job := metadata.SyncJob{
		ID:        randomID(),
		RuleID:    rule.ID,
		Status:    metadata.SyncJobRunning,
		StartedAt: time.Now().UTC(),
	}
	if err := s.meta.PutSyncJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	synced, errs, msg := s.runSyncJob(r.Context(), rule)
	now := time.Now().UTC()
	job.Objects = synced
	job.Errors = errs
	job.Message = msg
	job.Status = metadata.SyncJobCompleted
	if errs > 0 {
		job.Status = metadata.SyncJobFailed
	}
	job.EndedAt = &now
	_ = s.meta.PutSyncJob(job)
	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (s *Server) runSyncJob(ctx context.Context, rule metadata.ReplicationRule) (synced, errs int, msg string) {
	conn, err := s.meta.GetGatewayConnection(rule.DestConnection)
	if err != nil {
		if errors.Is(err, metadata.ErrNotFound) {
			return 0, 1, gatewayConnNotFoundErr(rule.DestConnection).Error()
		}
		return 0, 1, err.Error()
	}
	client, err := s.gatewayS3Client(conn)
	if err != nil {
		return 0, 1, err.Error()
	}
	if err := s.ensureRemoteBucket(ctx, client, rule.DestBucket, s.sourceBucketVisibility(rule.SourceBucket)); err != nil {
		return 0, 1, err.Error()
	}
	objs, err := s.meta.ListObjects(rule.SourceBucket, "", 0)
	if err != nil {
		return 0, 1, err.Error()
	}
	for _, obj := range objs {
		if obj.IsDeleteMarker || obj.Size == 0 {
			continue
		}
		rc, rec, err := s.svc.GetObject(ctx, rule.SourceBucket, obj.Key, "")
		if err != nil {
			if isLocalNotFound(err) {
				continue
			}
			errs++
			continue
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			errs++
			continue
		}
		ct := rec.ContentType
		if ct == "" {
			ct = "application/octet-stream"
		}
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(rule.DestBucket),
			Key:           aws.String(obj.Key),
			Body:          bytes.NewReader(body),
			ContentLength: aws.Int64(int64(len(body))),
			ContentType:   aws.String(ct),
		})
		if err != nil {
			errs++
			continue
		}
		synced++
	}
	return synced, errs, fmt.Sprintf("synced %d objects, %d errors", synced, errs)
}

func (s *Server) handleListSyncJobs(w http.ResponseWriter, r *http.Request) {
	ruleID := r.URL.Query().Get("rule_id")
	jobs, err := s.meta.ListSyncJobs(ruleID, 50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) handleGatewayHealth(w http.ResponseWriter, r *http.Request) {
	conns, _ := s.meta.ListGatewayConnections()
	rules, _ := s.meta.ListReplicationRules()
	jobs, _ := s.meta.ListSyncJobs("", 10)
	stats, _ := s.meta.GetGatewayStats()
	recentErrors, _ := s.meta.ListReplicationErrors(20)
	brokenRules, _ := s.meta.CountBrokenReplicationRules()
	okCount := 0
	for _, c := range conns {
		if c.Status == "ok" {
			okCount++
		}
	}
	publicReadRules := 0
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if s.sourceBucketVisibility(r.SourceBucket) == "public-read" {
			publicReadRules++
		}
	}
	lagSeconds := 0.0
	if !stats.OldestPending.IsZero() {
		lagSeconds = time.Since(stats.OldestPending).Seconds()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connections_total":  len(conns),
		"connections_ok":     okCount,
		"rules_total":        len(rules),
		"rules_broken":       brokenRules,
		"public_read_rules":  publicReadRules,
		"recent_jobs":        jobs,
		"recent_errors":      recentErrors,
		"queue_pending":      stats.PendingCount,
		"queue_lag_seconds":  lagSeconds,
		"bytes_replicated":   stats.BytesReplicated,
		"replication_errors": stats.ReplicationErrors,
		"tasks_completed":    stats.TasksCompletedTotal,
		"last_processed_at":  stats.LastProcessedAt,
	})
}

func (s *Server) handleListReplicationQueue(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = metadata.ReplTaskPending
	}
	tasks, err := s.meta.ListReplicationTasks(status, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": tasks})
}

func (s *Server) handleRetryFailedReplication(w http.ResponseWriter, r *http.Request) {
	count, err := s.meta.RetryFailedReplicationTasks()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"retried": count})
}

func (s *Server) handleClearReplicationErrors(w http.ResponseWriter, r *http.Request) {
	if err := s.meta.ClearReplicationErrors(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cleared": true})
}

func (s *Server) handleListFederationClusters(w http.ResponseWriter, r *http.Request) {
	clusters, err := s.meta.ListFederationClusters()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for i := range clusters {
		if len(clusters[i].Capabilities) == 0 {
			clusters[i].Capabilities = []string{"read", "list"}
		}
		status, _ := federation.TestConnectivity(clusters[i].Endpoint)
		clusters[i].Status = status
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": clusters})
}

func (s *Server) handleCreateFederationCluster(w http.ResponseWriter, r *http.Request) {
	var req metadata.FederationCluster
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	if req.Name == "" || req.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and endpoint required"})
		return
	}
	req.ID = randomID()
	req.CreatedAt = time.Now().UTC()
	req.Status = "registered"
	if len(req.Capabilities) == 0 {
		req.Capabilities = []string{"read", "list"}
	}
	if err := s.meta.PutFederationCluster(req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"cluster": req})
}

func (s *Server) handleDeleteFederationCluster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.meta.DeleteFederationCluster(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleFederationTestConnectivity(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cluster, err := s.meta.GetFederationCluster(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}
	status, detail := federation.TestConnectivity(cluster.Endpoint)
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "status": status, "detail": detail, "endpoint": cluster.Endpoint,
	})
}

func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.meta.GetSystemConfig()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	status, nodes := "healthy", cfg.Cluster.Nodes
	if s.cluster != nil {
		status, nodes = s.cluster.Snapshot()
	}
	if len(nodes) == 0 {
		nodes = []metadata.ClusterNode{{
			ID: "local", Address: "localhost:9000", Role: "primary", Status: "healthy",
		}}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":                 status,
		"distributed_mode":       cfg.Cluster.DistributedMode,
		"erasure_coding_planned": cfg.Cluster.ErasureCodingPlanned,
		"disk_paths":             cfg.Cluster.DiskPaths,
		"nodes":                  nodes,
	})
}

func (s *Server) mergeEnvConfig(cfg metadata.SystemConfig) metadata.SystemConfig {
	if v := os.Getenv("STORAGE_LDAP_URL"); v != "" && cfg.LDAP.URL == "" {
		cfg.LDAP.URL = v
	}
	if v := os.Getenv("STORAGE_LDAP_BIND_DN"); v != "" && cfg.LDAP.BindDN == "" {
		cfg.LDAP.BindDN = v
	}
	if v := os.Getenv("STORAGE_LDAP_BIND_PASSWORD"); v != "" && cfg.LDAP.BindPassword == "" {
		cfg.LDAP.BindPassword = v
	}
	if v := os.Getenv("STORAGE_LDAP_BASE_DN"); v != "" && cfg.LDAP.BaseDN == "" {
		cfg.LDAP.BaseDN = v
	}
	if v := os.Getenv("STORAGE_LDAP_ENABLED"); v == "true" || v == "1" {
		cfg.LDAP.Enabled = true
	}
	if v := os.Getenv("STORAGE_OIDC_ISSUER"); v != "" && cfg.OIDC.Issuer == "" {
		cfg.OIDC.Issuer = v
	}
	if v := os.Getenv("STORAGE_OIDC_INTERNAL_ISSUER"); v != "" && cfg.OIDC.InternalIssuer == "" {
		cfg.OIDC.InternalIssuer = v
	}
	if v := os.Getenv("STORAGE_OIDC_CLIENT_ID"); v != "" && cfg.OIDC.ClientID == "" {
		cfg.OIDC.ClientID = v
	}
	if v := os.Getenv("STORAGE_OIDC_CLIENT_SECRET"); v != "" && cfg.OIDC.ClientSecret == "" {
		cfg.OIDC.ClientSecret = v
	}
	if v := os.Getenv("STORAGE_OIDC_REDIRECT_URL"); v != "" && cfg.OIDC.RedirectURL == "" {
		cfg.OIDC.RedirectURL = v
	}
	if v := os.Getenv("STORAGE_OIDC_ENABLED"); v == "true" || v == "1" {
		cfg.OIDC.Enabled = true
	}
	return cfg
}
