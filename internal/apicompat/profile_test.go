package apicompat

import "testing"

func TestDetectAnonymousCapabilitiesDefersFeatures(t *testing.T) {
	body := []byte(`{"api":"v1","server_version":"1.6.1","spec_version":"1.6.1","login_methods":["password"]}`)
	got := DetectAnonymous(200, body, 200, []byte(`{"version":"1.5.0"}`))
	if got.Profile != ProfileCapabilities || !got.NeedsAuthForFeatures || got.ServerVersion != "1.6.1" {
		t.Fatalf("%+v", got)
	}
	if len(got.LoginMethods) != 1 || got.LoginMethods[0] != "password" {
		t.Fatalf("login methods %+v", got.LoginMethods)
	}
	if got.Features != nil {
		t.Fatalf("anonymous features should be absent, got %+v", got.Features)
	}
}

func TestApplyAuthenticatedFeatures(t *testing.T) {
	anon := DetectAnonymous(200, []byte(`{"api":"v1","server_version":"1.6.1","login_methods":["password","oidc"]}`), 0, nil)
	authBody := []byte(`{
	  "api":"v1","server_version":"1.6.1","spec_version":"1.6.1",
	  "login_methods":["password","oidc"],
	  "features":{"errors.codes":1,"auth.oidc":1,"sharing.policy":2},
	  "clients":{"desktop":{"min_supported":"1.0.0","min_required":null,"max_tested":null}},
	  "deprecations":[]
	}`)
	got := ApplyAuthenticated(anon, 200, authBody)
	if got.NeedsAuthForFeatures {
		t.Fatal("still waiting for auth")
	}
	if got.Features["errors.codes"] != 1 || got.Features["sharing.policy"] != 2 || got.Features["objects.multipart"] != 0 {
		t.Fatalf("features %+v", got.Features)
	}
	if got.MinSupported != "1.0.0" || got.MinRequired != "" {
		t.Fatalf("clients %+v", got)
	}
}

func TestHealthzFallback(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		profile string
	}{
		{"semver 1.6", `{"version":"1.6.0","kms":{"available":true}}`, Profile160},
		{"dev with kms is 1.6", `{"version":"dev","kms":{"available":true}}`, Profile160},
		{"dev without kms is 1.5", `{"version":"dev"}`, Profile150},
		{"1.5.0", `{"version":"1.5.0"}`, Profile150},
		{"1.4.1 rejected", `{"version":"1.4.1"}`, ProfileRejected},
		{"garbage basic", `{"version":"build-42"}`, ProfileBasic},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectAnonymous(404, []byte(`{"error":"not found"}`), 200, []byte(tc.body))
			if got.Profile != tc.profile {
				t.Fatalf("profile %s want %s (%+v)", got.Profile, tc.profile, got)
			}
		})
	}
	if got := DetectAnonymous(404, nil, 0, nil); got.Profile != ProfileBasic {
		t.Fatalf("unreachable health %+v", got)
	}
	got := DetectAnonymous(404, nil, 200, []byte(`{"version":"1.6.2","kms":{}}`))
	if got.Features["auth.step_up"] != 1 || got.Features["errors.codes"] != 0 {
		t.Fatalf("1.6 features %+v", got.Features)
	}
}

func TestDesktopDisposition(t *testing.T) {
	if got := DesktopDisposition("1.0.0", "1.0.0", "", "", "1.2.0"); got != DispositionOK {
		t.Fatalf("%s", got)
	}
	if got := DesktopDisposition("0.9.0", "1.0.0", "", "", ""); got != DispositionWarn {
		t.Fatalf("soft %s", got)
	}
	if got := DesktopDisposition("0.9.0", "1.0.0", "1.0.0", "security", ""); got != DispositionBlock {
		t.Fatalf("block %s", got)
	}
	if got := DesktopDisposition("0.9.0", "1.0.0", "1.0.0", "policy", ""); got != DispositionWarn {
		t.Fatalf("non-security min_required %s", got)
	}
	if got := DesktopDisposition("1.4.0", "1.0.0", "", "", "1.2.0"); got != DispositionUntested {
		t.Fatalf("untested %s", got)
	}
}
