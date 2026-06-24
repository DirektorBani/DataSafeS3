package auth

import (
	"os"
	"testing"
)

func TestAuthenticateLDAPGroupsIntegration(t *testing.T) {
	if os.Getenv("LDAP_INTEGRATION") != "1" {
		t.Skip("set LDAP_INTEGRATION=1 with datasafe-ldap-test on :389")
	}
	lu, err := AuthenticateLDAP(LDAPSettings{
		URL:          "ldap://localhost:389",
		BindDN:       "cn=admin,dc=datasafe,dc=local",
		BindPassword: "ldapadmin",
		BaseDN:       "ou=users,dc=datasafe,dc=local",
		GroupDN:      "ou=groups,dc=datasafe,dc=local",
	}, "ldapuser", "password")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if len(lu.Groups) == 0 {
		t.Fatalf("expected groups, got none")
	}
	found := false
	for _, g := range lu.Groups {
		if g == "datasafe-users" {
			found = true
		}
	}
	if !found {
		t.Fatalf("groups=%v want datasafe-users", lu.Groups)
	}
}
