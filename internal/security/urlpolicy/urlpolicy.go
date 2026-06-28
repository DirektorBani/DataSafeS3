// Package urlpolicy validates outbound HTTP(S) URLs to mitigate SSRF.
package urlpolicy

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// Options controls outbound URL validation.
type Options struct {
	// AllowHTTP permits http:// when false only https:// is allowed.
	AllowHTTP bool
	// AllowPrivate permits RFC1918, loopback, link-local, and metadata IPs.
	AllowPrivate bool
	// BlockLocalhost rejects hostname localhost (independent of IP resolution).
	BlockLocalhost bool
}

// DefaultOptions returns production-safe defaults. Dev mode relaxes private IP and HTTP checks.
func DefaultOptions() Options {
	dev := isDevMode()
	allowHTTP := dev || os.Getenv("STORAGE_OUTBOUND_HTTP_ALLOW") == "true"
	return Options{
		AllowHTTP:      allowHTTP,
		AllowPrivate:   dev || allowHTTP,
		BlockLocalhost: !dev,
	}
}

func isDevMode() bool {
	switch os.Getenv("STORAGE_DEV") {
	case "1", "true", "yes":
		return true
	}
	return false
}

// ValidateOutboundURL checks that raw is a safe outbound target under opts.
func ValidateOutboundURL(raw string, opts Options) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty url")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("url scheme required")
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "https":
	case "http":
		if !opts.AllowHTTP {
			return fmt.Errorf("http not allowed in strict mode")
		}
	default:
		return fmt.Errorf("scheme %q not allowed", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("host required")
	}
	if opts.BlockLocalhost && isLocalhostName(host) {
		return fmt.Errorf("localhost not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !opts.AllowPrivate && isBlockedIP(ip) {
			return fmt.Errorf("private or reserved IP not allowed")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		// Unresolvable hostnames are rejected in strict mode (no DNS rebinding bypass).
		if !opts.AllowPrivate {
			return fmt.Errorf("hostname resolution failed: %w", err)
		}
		return nil
	}
	if !opts.AllowPrivate {
		for _, ip := range ips {
			if isBlockedIP(ip) {
				return fmt.Errorf("hostname resolves to private or reserved IP")
			}
		}
	}
	return nil
}

func isLocalhostName(host string) bool {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	return h == "localhost"
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() {
		return true
	}
	// AWS/GCP metadata endpoints
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
	}
	return false
}

// OutboundURLError formats a user-facing validation error.
func OutboundURLError(err error) string {
	if err == nil {
		return ""
	}
	return "outbound url not allowed: " + err.Error()
}
