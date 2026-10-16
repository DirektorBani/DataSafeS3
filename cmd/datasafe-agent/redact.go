package main

import "strings"

var secretKeys = []string{
	"password", "secret", "token", "mfa_token", "mfa_code", "exchange_code",
	"authorization", "access_key_secret",
}

// redactMap copies params for logs. Secret fields become a fixed marker.
func redactMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if isSecretKey(k) {
			out[k] = "[redacted]"
			continue
		}
		if child, ok := v.(map[string]any); ok {
			out[k] = redactMap(child)
			continue
		}
		out[k] = v
	}
	return out
}

func isSecretKey(k string) bool {
	k = strings.ToLower(k)
	for _, secret := range secretKeys {
		if k == secret || strings.Contains(k, secret) {
			return true
		}
	}
	return false
}

// lineHasSecret reports whether a log line kept a caller-supplied secret.
func lineHasSecret(line string, secrets ...string) bool {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(line, secret) {
			return true
		}
	}
	return false
}
