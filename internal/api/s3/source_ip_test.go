package s3

import (
	"net/http"
	"testing"
)

func TestRequestSourceIPIgnoresSpoofedForwardedFor(t *testing.T) {
	t.Setenv("STORAGE_TRUSTED_PROXIES", "")
	req, _ := http.NewRequest(http.MethodGet, "http://example/", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	if got := requestSourceIP(req); got != "203.0.113.9" {
		t.Fatalf("source %s", got)
	}
}

func TestRequestSourceIPTrustsProxyChain(t *testing.T) {
	t.Setenv("STORAGE_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.10")
	req, _ := http.NewRequest(http.MethodGet, "http://example/", nil)
	req.RemoteAddr = "10.1.2.3:443"
	req.Header.Set("X-Forwarded-For", "198.51.100.2, 10.9.9.9")
	if got := requestSourceIP(req); got != "198.51.100.2" {
		t.Fatalf("source %s", got)
	}
	req.RemoteAddr = "203.0.113.8:443"
	req.Header.Set("X-Forwarded-For", "198.51.100.2")
	if got := requestSourceIP(req); got != "203.0.113.8" {
		t.Fatalf("untrusted peer source %s", got)
	}
}
