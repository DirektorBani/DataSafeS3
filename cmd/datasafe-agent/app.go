package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// App is the stdio agent. It keeps profiles on disk and secrets only in
// the RPC call that is using them.
type App struct {
	dataDir      string
	log          *Logger
	transferOnce sync.Once
	transfers    *Engine
	transferErr  error
}

func (a *App) inspect(profile Profile, token string) (ServerCard, error) {
	client, err := clientFor(profile)
	if err != nil {
		return ServerCard{}, err
	}
	public, publicReason := a.readCaps(client, "")
	var authed *capsInfo
	authedReason := ""
	reauth := false
	if token != "" {
		client.Token = token
		got, reason := a.readCaps(client, token)
		authedReason = reason
		if got.Status == http.StatusUnauthorized {
			reauth = true
		} else {
			authed = got
		}
		client.Token = ""
	}
	health, meReached, healthReason := a.readHealth(client)
	card := detect(public, authed, health, meReached, DesktopVersion, preferNetReason(publicReason, authedReason, healthReason))
	card.Reauth = reauth
	if profile.ExpiresAtUnix > 0 {
		left := time.Until(time.Unix(profile.ExpiresAtUnix, 0))
		card.ExpiresSoon = left > 0 && left <= 5*time.Minute
	}
	if err := a.writeCache(profile.ID, card); err != nil {
		return ServerCard{}, err
	}
	profile.Server = &card
	switch {
	case card.Rejected:
		profile.Session = "rejected"
	case card.Blocked:
		profile.Session = "blocked"
	case reauth:
		profile.Session = "reauth"
	}
	_ = a.updateProfile(profile)
	return card, nil
}

func (a *App) readCaps(client *Client, token string) (*capsInfo, string) {
	client.Token = token
	resp, err := client.do(http.MethodGet, "/api/v1/capabilities", nil)
	info := &capsInfo{Reached: err == nil}
	if err != nil {
		return info, netReason(err)
	}
	info.Status = resp.Status
	if resp.Header.Get("Deprecation") != "" && info.Status == http.StatusOK {
		// Recorded below when the body has no deprecations list.
	}
	obj, ok := decodeObject(resp.Body)
	if !ok {
		return info, ""
	}
	api, _ := obj["api"].(string)
	info.API = api
	info.ServerVersion, _ = obj["server_version"].(string)
	info.SpecVersion, _ = obj["spec_version"].(string)
	if methods, ok := obj["login_methods"].([]any); ok {
		for _, item := range methods {
			if s, ok := item.(string); ok {
				info.LoginMethods = append(info.LoginMethods, s)
			}
		}
	}
	if raw, exists := obj["features"]; exists && raw != nil {
		if features, ok := raw.(map[string]any); ok {
			info.HasFeatures = true
			info.Features = map[string]int{}
			for key, value := range features {
				switch n := value.(type) {
				case float64:
					if n >= 1 {
						info.Features[key] = int(n)
					}
				case json.Number:
					if i, err := n.Int64(); err == nil && i >= 1 {
						info.Features[key] = int(i)
					}
				}
			}
		}
	}
	if clients, ok := obj["clients"].(map[string]any); ok {
		if desktop, ok := clients["desktop"].(map[string]any); ok {
			info.Desktop = desktop
		}
	}
	if items, ok := obj["deprecations"].([]any); ok {
		for _, item := range items {
			if row, ok := item.(map[string]any); ok {
				info.Deprecations = append(info.Deprecations, row)
			}
		}
	}
	if info.Deprecations == nil && resp.Header.Get("Deprecation") != "" {
		info.Deprecations = []map[string]any{{
			"method": "",
			"path":   "Deprecation",
		}}
	}
	return info, ""
}

func (a *App) readHealth(client *Client) (healthInfo, bool, string) {
	client.Token = ""
	resp, err := client.do(http.MethodGet, "/healthz", nil)
	reason := netReason(err)
	health := healthInfo{}
	if err == nil && resp.Status == http.StatusOK {
		health.Reached = true
		if obj, ok := decodeObject(resp.Body); ok {
			health.Version, _ = obj["version"].(string)
			if _, ok := obj["kms"]; ok {
				health.HasKMS = true
			}
		}
	}
	me, err := client.do(http.MethodGet, "/api/v1/me", nil)
	if reason == "" {
		reason = netReason(err)
	}
	meReached := err == nil && me.Status != 0
	return health, meReached, reason
}

type loginResult struct {
	Status          string     `json:"status"`
	Username        string     `json:"username,omitempty"`
	Role            string     `json:"role,omitempty"`
	Token           string     `json:"token,omitempty"`
	MFAToken        string     `json:"mfa_token,omitempty"`
	ExpiresIn       int        `json:"expires_in,omitempty"`
	MFASetup        bool       `json:"mfa_setup"`
	PasswordRevoked bool       `json:"password_revoked"`
	Message         string     `json:"message,omitempty"`
	Server          ServerCard `json:"server"`
}

func (a *App) loginPassword(profile Profile, username, password string) (loginResult, error) {
	if username == "" {
		username = profile.Username
	}
	return a.login(profile, "/api/v1/admin/login", map[string]string{
		"username": username,
		"password": password,
	}, false)
}

func (a *App) loginOIDCPassword(profile Profile, username, password string) (loginResult, error) {
	return a.login(profile, "/api/v1/auth/oidc/password-login", map[string]string{
		"username": username,
		"password": password,
	}, true)
}

func (a *App) loginOIDCExchange(profile Profile, code string) (loginResult, error) {
	return a.login(profile, "/api/v1/auth/oidc/exchange", map[string]string{
		"exchange_code": code,
	}, true)
}

func (a *App) loginMFA(profile Profile, mfaToken, code string) (loginResult, error) {
	return a.login(profile, "/api/v1/mfa/login", map[string]string{
		"mfa_token": mfaToken,
		"mfa_code":  code,
	}, false)
}

func (a *App) acceptToken(profile Profile, token string, expiresIn int) (loginResult, error) {
	if strings.TrimSpace(token) == "" {
		return loginResult{}, fmt.Errorf("token is required")
	}
	if expiresIn <= 0 {
		expiresIn = 86400
	}
	return a.finishLogin(profile, loginResult{
		Status:    "signed_in",
		Token:     token,
		ExpiresIn: expiresIn,
	})
}

func (a *App) login(profile Profile, path string, payload map[string]string, oidc bool) (loginResult, error) {
	card, err := a.inspect(profile, "")
	if err != nil {
		return loginResult{}, err
	}
	if card.Rejected {
		return loginResult{Status: "rejected", Message: card.Warning, Server: card}, nil
	}
	if card.Blocked {
		return loginResult{Status: "blocked", Message: card.Warning, Server: card}, nil
	}
	client, err := clientFor(profile)
	if err != nil {
		return loginResult{}, err
	}
	resp, err := client.do(http.MethodPost, path, payload)
	if err != nil {
		return loginResult{}, err
	}
	obj, ok := decodeObject(resp.Body)
	if resp.Status != http.StatusOK || !ok {
		apiErr := classifyError(resp.Status, resp.Body, resp.Header)
		return loginResult{Status: "error", Message: safeMessage(apiErr), Server: card}, nil
	}
	if required, _ := obj["mfa_required"].(bool); required {
		mfaToken, _ := obj["mfa_token"].(string)
		return loginResult{Status: "mfa_required", MFAToken: mfaToken, Server: card}, nil
	}
	token, _ := obj["token"].(string)
	if token == "" {
		return loginResult{Status: "error", Message: "login did not return a session", Server: card}, nil
	}
	username, _ := obj["username"].(string)
	role, _ := obj["role"].(string)
	expires := 86400
	if n, ok := obj["expires_in"].(float64); ok && n > 0 {
		expires = int(n)
	}
	setup, _ := obj["mfa_setup_required"].(bool)
	result := loginResult{
		Status:    "signed_in",
		Username:  username,
		Role:      role,
		Token:     token,
		ExpiresIn: expires,
		MFASetup:  setup,
		Server:    card,
	}
	if oidc {
		result.PasswordRevoked = true
	}
	return a.finishLogin(profile, result)
}

func (a *App) finishLogin(profile Profile, result loginResult) (loginResult, error) {
	profile.ExpiresAtUnix = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Unix()
	profile.Session = "signed_in"
	card, err := a.inspect(profile, result.Token)
	if err != nil {
		return loginResult{}, err
	}
	result.Server = card
	if card.Blocked {
		result.Status = "blocked"
		result.Message = card.Warning
	}
	if card.Reauth {
		result.Status = "reauth"
	}
	profile, err = a.profileByID(profile.ID)
	if err != nil {
		return loginResult{}, err
	}
	profile.Session = result.Status
	profile.ExpiresAtUnix = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Unix()
	if err := a.updateProfile(profile); err != nil {
		return loginResult{}, err
	}
	return result, nil
}

func (a *App) logout(profile Profile, token string) (loginResult, error) {
	if eng, err := a.engine(); err == nil && eng != nil {
		_ = eng.PauseProfile(profile.ID)
	}
	resume := func() {
		if strings.TrimSpace(token) == "" {
			return
		}
		if eng, err := a.engine(); err == nil && eng != nil {
			_, _ = eng.Resume(profile, token, "", "")
		}
	}
	revoked := false
	if token != "" && (profile.Server == nil || !profile.Server.Blocked) {
		client, err := clientFor(profile)
		if err != nil {
			resume()
			return loginResult{}, err
		}
		client.Token = token
		resp, err := client.do(http.MethodPost, "/api/v1/admin/logout", map[string]any{})
		if err != nil {
			resume()
			return loginResult{}, err
		}
		revoked = resp.Status == http.StatusNoContent || resp.Status == http.StatusOK
		if profile.AuthMethod == "password" {
			revoked = false
		}
	}
	profile.Session = "signed_out"
	profile.ExpiresAtUnix = 0
	if err := a.updateProfile(profile); err != nil {
		resume()
		return loginResult{}, err
	}
	message := ""
	if profile.AuthMethod == "password" {
		message = "the server does not revoke a password session before it expires"
	}
	return loginResult{Status: "signed_out", PasswordRevoked: revoked, Message: message}, nil
}

// probe calls GET /api/v1/me. A 401 that is not step-up asks for one fresh
// login. The caller retries once; a second 401 is a login error.
func (a *App) probe(profile Profile, token string, retry bool) (loginResult, error) {
	if profile.Server != nil && profile.Server.Blocked {
		return loginResult{Status: "blocked", Message: profile.Server.Warning, Server: *profile.Server}, nil
	}
	if profile.Server != nil && profile.Server.Rejected {
		return loginResult{Status: "rejected", Message: profile.Server.Warning, Server: *profile.Server}, nil
	}
	client, err := clientFor(profile)
	if err != nil {
		return loginResult{}, err
	}
	client.Token = token
	resp, err := client.do(http.MethodGet, "/api/v1/me", nil)
	if err != nil {
		return loginResult{}, err
	}
	if resp.Status == http.StatusOK {
		obj, _ := decodeObject(resp.Body)
		username, _ := obj["username"].(string)
		setup, _ := obj["mfa_setup_required"].(bool)
		profile.Session = "signed_in"
		_ = a.updateProfile(profile)
		return loginResult{Status: "signed_in", Username: username, MFASetup: setup}, nil
	}
	apiErr := classifyError(resp.Status, resp.Body, resp.Header)
	if apiErr.StepUp {
		return loginResult{Status: "step_up", Message: safeMessage(apiErr)}, nil
	}
	if apiErr.MFASetup {
		return loginResult{Status: "signed_in", MFASetup: true, Message: "set up 2FA in the web console"}, nil
	}
	if reauthCode(apiErr) {
		status := "reauth"
		if retry {
			status = "login_error"
		}
		profile.Session = status
		_ = a.updateProfile(profile)
		return loginResult{Status: status, Message: safeMessage(apiErr)}, nil
	}
	return loginResult{Status: "error", Message: safeMessage(apiErr)}, nil
}

func safeMessage(err apiError) string {
	if err.Code != "" {
		return err.Code
	}
	return "request failed"
}
