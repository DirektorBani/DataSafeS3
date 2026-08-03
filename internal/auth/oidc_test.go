package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/oauth2"
)

func TestRewriteEndpointHost(t *testing.T) {
	t.Parallel()
	cases := []struct {
		endpoint string
		issuer   string
		want     string
	}{
		{
			endpoint: "http://localhost:8180/realms/datasafe/protocol/openid-connect/token",
			issuer:   "http://host.docker.internal:8180/realms/datasafe",
			want:     "http://host.docker.internal:8180/realms/datasafe/protocol/openid-connect/token",
		},
		{
			endpoint: "http://127.0.0.1:8180/realms/datasafe/protocol/openid-connect/auth",
			issuer:   "http://host.docker.internal:8180/realms/datasafe",
			want:     "http://host.docker.internal:8180/realms/datasafe/protocol/openid-connect/auth",
		},
		{
			endpoint: "",
			issuer:   "http://host.docker.internal:8180/realms/datasafe",
			want:     "",
		},
	}
	for _, tc := range cases {
		if got := RewriteEndpointHost(tc.endpoint, tc.issuer); got != tc.want {
			t.Fatalf("RewriteEndpointHost(%q, %q) = %q, want %q", tc.endpoint, tc.issuer, got, tc.want)
		}
	}
}

func TestResolveOIDCIssuersExplicitInternal(t *testing.T) {
	t.Parallel()
	got := ResolveOIDCIssuers("http://localhost:8180/realms/datasafe", "http://host.docker.internal:8180/realms/datasafe")
	if got.Public != "http://localhost:8180/realms/datasafe" {
		t.Fatalf("public = %q", got.Public)
	}
	if got.Internal != "http://host.docker.internal:8180/realms/datasafe" {
		t.Fatalf("internal = %q", got.Internal)
	}
}

func TestServerOAuthEndpointRewritesDiscoveryLocalhost(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/realms/datasafe/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
			"authorization_endpoint":"http://localhost:8180/realms/datasafe/protocol/openid-connect/auth",
			"token_endpoint":"http://localhost:8180/realms/datasafe/protocol/openid-connect/token",
			"userinfo_endpoint":"http://localhost:8180/realms/datasafe/protocol/openid-connect/userinfo"
		}`))
	}))
	defer srv.Close()

	issuers := OIDCIssuers{
		Public:   "http://localhost:8180/realms/datasafe",
		Internal: srv.URL + "/realms/datasafe",
	}
	ep := ServerOAuthEndpoint(issuers, srv.Client())
	wantToken := issuers.Internal + "/protocol/openid-connect/token"
	if ep.TokenURL != wantToken {
		t.Fatalf("TokenURL = %q, want %q", ep.TokenURL, wantToken)
	}
}

func TestBrowserOAuthEndpointKeepsPublicAuthURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"authorization_endpoint":"http://localhost:8180/realms/datasafe/protocol/openid-connect/auth",
			"token_endpoint":"http://localhost:8180/realms/datasafe/protocol/openid-connect/token"
		}`))
	}))
	defer srv.Close()

	issuers := OIDCIssuers{
		Public:   srv.URL + "/realms/datasafe",
		// Closed port so Internal discovery fails even when a local Keycloak is up.
		Internal: "http://127.0.0.1:1/realms/datasafe",
	}
	ep := BrowserOAuthEndpoint(issuers, srv.Client())
	wantAuth := "http://localhost:8180/realms/datasafe/protocol/openid-connect/auth"
	if ep.AuthURL != wantAuth {
		t.Fatalf("AuthURL = %q, want %q", ep.AuthURL, wantAuth)
	}
}

func TestBrowserOAuthEndpointRewritesInternalDiscoveryToPublic(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"authorization_endpoint":"http://host.docker.internal:8180/realms/datasafe/protocol/openid-connect/auth",
			"token_endpoint":"http://host.docker.internal:8180/realms/datasafe/protocol/openid-connect/token"
		}`))
	}))
	defer srv.Close()

	issuers := OIDCIssuers{
		Public:   "http://localhost:8180/realms/datasafe",
		Internal: srv.URL + "/realms/datasafe",
	}
	ep := BrowserOAuthEndpoint(issuers, srv.Client())
	want := "http://localhost:8180/realms/datasafe/protocol/openid-connect/auth"
	if ep.AuthURL != want {
		t.Fatalf("AuthURL = %q, want %q", ep.AuthURL, want)
	}
}

func TestProbeOIDCDiscoveryPrefersInternal(t *testing.T) {
	t.Parallel()
	internalOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"authorization_endpoint":"http://x/auth","token_endpoint":"http://x/token"}`))
	}))
	defer internalOK.Close()

	ok, err := ProbeOIDCDiscovery(OIDCIssuers{
		Public:   "http://127.0.0.1:1/realms/missing",
		Internal: internalOK.URL + "/realms/datasafe",
	}, internalOK.Client())
	if err != nil || !ok {
		t.Fatalf("expected reachable via internal, ok=%v err=%v", ok, err)
	}

	ok, err = ProbeOIDCDiscovery(OIDCIssuers{
		Public:   "http://127.0.0.1:1/realms/missing",
		Internal: "http://127.0.0.1:1/realms/also-missing",
	}, http.DefaultClient)
	if ok || err == nil {
		t.Fatalf("expected unreachable, ok=%v err=%v", ok, err)
	}
}

func TestParseOIDCUserFromAccessTokenJWT(t *testing.T) {
	t.Parallel()
	// payload: {"preferred_username":"ssouser","email":"ssouser@datasafe.local","sub":"abc"}
	payload := "eyJhbGciOiJub25lIn0.eyJwcmVmZXJyZWRfdXNlcm5hbWUiOiJzc291c2VyIiwiZW1haWwiOiJzc291c2VyQGRhdGFzYWZlLmxvY2FsIiwic3ViIjoiYWJjIn0."
	tok := &oauth2.Token{AccessToken: payload + "sig"}
	user, email, err := ParseOIDCUserFromToken(tok, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if user != "ssouser" || email != "ssouser@datasafe.local" {
		t.Fatalf("got %q / %q", user, email)
	}
}

func TestServerUserInfoURLRewritesHost(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"userinfo_endpoint":"http://localhost:8180/realms/datasafe/protocol/openid-connect/userinfo"
		}`))
	}))
	defer srv.Close()

	issuers := OIDCIssuers{Internal: srv.URL + "/realms/datasafe"}
	got := ServerUserInfoURL(issuers, srv.Client())
	want := issuers.Internal + "/protocol/openid-connect/userinfo"
	if got != want {
		t.Fatalf("UserInfoURL = %q, want %q", got, want)
	}
}
