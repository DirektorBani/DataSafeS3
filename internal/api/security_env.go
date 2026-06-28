package api

import (
	"fmt"
	"os"
	"strings"
)

func oidcROPCEnabled() bool {
	v := os.Getenv("STORAGE_OIDC_ROPC_ENABLED")
	if v == "" {
		return true
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func ldapTLSRequired() bool {
	return os.Getenv("STORAGE_LDAP_REQUIRE_TLS") == "true"
}

func storageDevMode() bool {
	switch os.Getenv("STORAGE_DEV") {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func validateLDAPTLS(rawURL string) error {
	if !ldapTLSRequired() {
		return nil
	}
	u := strings.ToLower(strings.TrimSpace(rawURL))
	if strings.HasPrefix(u, "ldap://") {
		return fmt.Errorf("ldap:// not allowed when STORAGE_LDAP_REQUIRE_TLS=true; use ldaps://")
	}
	return nil
}
