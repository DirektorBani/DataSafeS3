package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// OIDCIssuers separates browser-facing and server-side IdP base URLs.
// Public is used for authorization redirects; Internal for token exchange and userinfo.
type OIDCIssuers struct {
	Public   string
	Internal string
}

// ResolveOIDCIssuers picks internal issuer from explicit config or auto-derives it in Docker.
func ResolveOIDCIssuers(public, internal string) OIDCIssuers {
	public = strings.TrimSpace(public)
	internal = strings.TrimSpace(internal)
	if internal == "" {
		internal = DeriveInternalIssuer(public)
	}
	if internal == "" {
		internal = public
	}
	return OIDCIssuers{Public: public, Internal: internal}
}

// DeriveInternalIssuer rewrites localhost loopback hosts to host.docker.internal when running in a container.
func DeriveInternalIssuer(public string) string {
	if !RunningInDocker() {
		return public
	}
	u, err := url.Parse(public)
	if err != nil {
		return public
	}
	h := strings.ToLower(u.Hostname())
	if h != "localhost" && h != "127.0.0.1" && h != "[::1]" {
		return public
	}
	if port := u.Port(); port != "" {
		u.Host = "host.docker.internal:" + port
	} else {
		u.Host = "host.docker.internal"
	}
	return u.String()
}

// RunningInDocker reports whether the process appears to run inside a container.
func RunningInDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// RewriteEndpointHost replaces the scheme/host of endpoint with those from issuerBase.
// Keycloak discovery may advertise localhost even when queried via host.docker.internal.
func RewriteEndpointHost(endpoint, issuerBase string) string {
	endpoint = strings.TrimSpace(endpoint)
	issuerBase = strings.TrimSpace(issuerBase)
	if endpoint == "" || issuerBase == "" {
		return endpoint
	}
	ep, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	base, err := url.Parse(strings.TrimSuffix(issuerBase, "/"))
	if err != nil {
		return endpoint
	}
	ep.Scheme = base.Scheme
	ep.Host = base.Host
	return ep.String()
}

func defaultOIDCEndpoint(issuer string) oauth2.Endpoint {
	base := strings.TrimSuffix(issuer, "/")
	return oauth2.Endpoint{
		AuthURL:  base + "/protocol/openid-connect/auth",
		TokenURL: base + "/protocol/openid-connect/token",
	}
}

type oidcDiscovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// DiscoverOIDCEndpoints loads OpenID Provider metadata for the given issuer base URL.
func DiscoverOIDCEndpoints(issuer string, client *http.Client) (authURL, tokenURL, userInfoURL string, err error) {
	issuer = strings.TrimSuffix(strings.TrimSpace(issuer), "/")
	if issuer == "" {
		return "", "", "", nil
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return "", "", "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()
	var doc oidcDiscovery
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", "", "", err
	}
	return doc.AuthorizationEndpoint, doc.TokenEndpoint, doc.UserinfoEndpoint, nil
}

// BrowserOAuthEndpoint returns OAuth2 endpoints for browser authorization redirects.
func BrowserOAuthEndpoint(issuers OIDCIssuers, client *http.Client) oauth2.Endpoint {
	ep := defaultOIDCEndpoint(issuers.Public)
	authURL, _, _, err := DiscoverOIDCEndpoints(issuers.Public, client)
	if err == nil && authURL != "" {
		ep.AuthURL = authURL
	}
	return ep
}

// ServerOAuthEndpoint returns OAuth2 endpoints for server-side token exchange.
func ServerOAuthEndpoint(issuers OIDCIssuers, client *http.Client) oauth2.Endpoint {
	ep := defaultOIDCEndpoint(issuers.Internal)
	authURL, tokenURL, _, err := DiscoverOIDCEndpoints(issuers.Internal, client)
	if err == nil {
		if authURL != "" {
			ep.AuthURL = RewriteEndpointHost(authURL, issuers.Internal)
		}
		if tokenURL != "" {
			ep.TokenURL = RewriteEndpointHost(tokenURL, issuers.Internal)
		}
	}
	return ep
}

// ServerUserInfoURL returns the userinfo endpoint reachable from the storage-server.
func ServerUserInfoURL(issuers OIDCIssuers, client *http.Client) string {
	fallback := strings.TrimSuffix(issuers.Internal, "/") + "/protocol/openid-connect/userinfo"
	_, _, userInfo, err := DiscoverOIDCEndpoints(issuers.Internal, client)
	if err != nil || userInfo == "" {
		return fallback
	}
	return RewriteEndpointHost(userInfo, issuers.Internal)
}

// ParseOIDCUserFromToken extracts username and email from token response claims.
// Prefers id_token/access_token JWT payloads to avoid userinfo calls that may fail
// when Keycloak issuer hostname differs from the internal Docker reachability host.
func ParseOIDCUserFromToken(token *oauth2.Token, userInfoURL string, client *http.Client) (username, email string, err error) {
	if token == nil {
		return "", "", errors.New("missing token")
	}
	if id, ok := token.Extra("id_token").(string); ok && id != "" {
		if username, email, err := parseOIDCJWTClaims(id); err == nil {
			return username, email, nil
		}
	}
	if token.AccessToken != "" {
		if username, email, err := parseOIDCJWTClaims(token.AccessToken); err == nil {
			return username, email, nil
		}
	}
	return fetchOIDCUserInfo(token.AccessToken, userInfoURL, client)
}

func parseOIDCJWTClaims(tokenStr string) (username, email string, err error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) < 2 {
		return "", "", errors.New("invalid jwt")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", err
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", "", err
	}
	if v, ok := claims["preferred_username"].(string); ok && v != "" {
		username = v
	} else if v, ok := claims["sub"].(string); ok && v != "" {
		username = v
	}
	if v, ok := claims["email"].(string); ok {
		email = v
	}
	if username == "" {
		return "", "", errors.New("no username in oidc token")
	}
	if email == "" {
		email = username + "@oidc.local"
	}
	return username, email, nil
}

// ParseOIDCGroupsFromToken extracts group names from OIDC token claims.
func ParseOIDCGroupsFromToken(token *oauth2.Token, claimName string, userInfoURL string, client *http.Client) []string {
	if claimName == "" {
		claimName = "groups"
	}
	if token != nil {
		if id, ok := token.Extra("id_token").(string); ok && id != "" {
			if groups := parseOIDCGroupsFromJWT(id, claimName); len(groups) > 0 {
				return groups
			}
		}
		if token.AccessToken != "" {
			if groups := parseOIDCGroupsFromJWT(token.AccessToken, claimName); len(groups) > 0 {
				return groups
			}
		}
	}
	if token != nil && token.AccessToken != "" && userInfoURL != "" {
		if groups := fetchOIDCGroupsFromUserInfo(token.AccessToken, userInfoURL, claimName, client); len(groups) > 0 {
			return groups
		}
	}
	return nil
}

func parseOIDCGroupsFromJWT(tokenStr, claimName string) []string {
	parts := strings.Split(tokenStr, ".")
	if len(parts) < 2 {
		return nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	return groupsFromClaims(claims, claimName)
}

func fetchOIDCGroupsFromUserInfo(accessToken, userInfoURL, claimName string, client *http.Client) []string {
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var claims map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&claims); err != nil {
		return nil
	}
	return groupsFromClaims(claims, claimName)
}

func groupsFromClaims(claims map[string]any, claimName string) []string {
	raw, ok := claims[claimName]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	case string:
		if v != "" {
			return []string{v}
		}
	}
	return nil
}

func fetchOIDCUserInfo(accessToken, userInfoURL string, client *http.Client) (username, email string, err error) {
	if accessToken == "" || userInfoURL == "" {
		return "", "", errors.New("no username in oidc profile")
	}
	if client == nil {
		client = http.DefaultClient
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", errors.New("userinfo request failed")
	}
	var info map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", "", err
	}
	if v, ok := info["preferred_username"].(string); ok && v != "" {
		username = v
	} else if v, ok := info["sub"].(string); ok {
		username = v
	}
	if v, ok := info["email"].(string); ok {
		email = v
	}
	if username == "" {
		return "", "", errors.New("no username in oidc profile")
	}
	if email == "" {
		email = username + "@oidc.local"
	}
	return username, email, nil
}
