package urlpolicy

import (
	"strings"
	"testing"
)

func strictOpts() Options {
	return Options{AllowHTTP: false, AllowPrivate: false, BlockLocalhost: true}
}

func devOpts() Options {
	return Options{AllowHTTP: true, AllowPrivate: true, BlockLocalhost: false}
}

func TestValidateOutboundURL_metadataIP(t *testing.T) {
	err := ValidateOutboundURL("http://169.254.169.254/", strictOpts())
	if err == nil {
		t.Fatal("expected error for metadata IP")
	}
}

func TestValidateOutboundURL_rfc1918(t *testing.T) {
	cases := []string{
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://127.0.0.1:3100/",
	}
	for _, u := range cases {
		if err := ValidateOutboundURL(u, strictOpts()); err == nil {
			t.Fatalf("%s should be blocked in strict mode", u)
		}
	}
}

func TestValidateOutboundURL_validPublicHTTPS(t *testing.T) {
	cases := []string{
		"https://hooks.slack.com/services/XXX/YYY/ZZZ",
		"https://discord.com/api/webhooks/ID/TOKEN",
		"https://example.com/path",
	}
	for _, u := range cases {
		if err := ValidateOutboundURL(u, strictOpts()); err != nil {
			t.Fatalf("%s: %v", u, err)
		}
	}
}

func TestValidateOutboundURL_localhost(t *testing.T) {
	if err := ValidateOutboundURL("http://localhost:3100/", strictOpts()); err == nil {
		t.Fatal("localhost should be blocked")
	}
}

func TestValidateOutboundURL_devAllowsPrivate(t *testing.T) {
	if err := ValidateOutboundURL("http://127.0.0.1:3100/", devOpts()); err != nil {
		t.Fatalf("dev mode: %v", err)
	}
}

func TestValidateOutboundURL_httpStrict(t *testing.T) {
	err := ValidateOutboundURL("http://hooks.slack.com/", strictOpts())
	if err == nil || !strings.Contains(err.Error(), "http not allowed") {
		t.Fatalf("expected http rejection, got %v", err)
	}
}

func TestValidateOutboundURL_invalidScheme(t *testing.T) {
	if err := ValidateOutboundURL("file:///etc/passwd", strictOpts()); err == nil {
		t.Fatal("file scheme should be blocked")
	}
}

func TestValidateOutboundURL_idnHostname(t *testing.T) {
	// Punycode public hostname — should parse without error (may fail DNS in CI).
	u := "https://xn--bcher-kva.example/"
	err := ValidateOutboundURL(u, strictOpts())
	if err != nil && !strings.Contains(err.Error(), "resolution failed") {
		t.Fatalf("unexpected idn error: %v", err)
	}
}

func TestDefaultOptions_dev(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	opts := DefaultOptions()
	if !opts.AllowPrivate || !opts.AllowHTTP {
		t.Fatalf("dev defaults: %+v", opts)
	}
}

func TestDefaultOptions_prodMode_matrix(t *testing.T) {
	loopback := "http://127.0.0.1:3100/"
	cases := []struct {
		name      string
		dev       string
		allowHTTP string
		url       string
		wantAllow bool
	}{
		{"prod blocks loopback", "false", "", loopback, false},
		{"prod blocks plain http public", "false", "", "http://hooks.slack.com/", false},
		{"unset dev blocks loopback", "", "", loopback, false},
		{"empty allow blocks loopback", "false", "", loopback, false},
		{"explicit false allow blocks loopback", "false", "false", loopback, false},
		{"allow true permits loopback http", "false", "true", loopback, true},
		{"dev permits loopback without allow flag", "true", "", loopback, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("STORAGE_DEV", tc.dev)
			t.Setenv("STORAGE_OUTBOUND_HTTP_ALLOW", tc.allowHTTP)
			opts := DefaultOptions()
			err := ValidateOutboundURL(tc.url, opts)
			if tc.wantAllow && err != nil {
				t.Fatalf("expected allow: %v", err)
			}
			if !tc.wantAllow && err == nil {
				t.Fatal("expected block")
			}
		})
	}
}
