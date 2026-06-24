package auth

import (
	"testing"
)

func TestResolveLDAPURL(t *testing.T) {
	t.Parallel()
	if RunningInDocker() {
		got := ResolveLDAPURL("ldap://localhost:389")
		want := "ldap://host.docker.internal:389"
		if got != want {
			t.Fatalf("ResolveLDAPURL(localhost) = %q, want %q", got, want)
		}
		got = ResolveLDAPURL("ldap://127.0.0.1:389")
		if got != want {
			t.Fatalf("ResolveLDAPURL(127.0.0.1) = %q, want %q", got, want)
		}
	} else {
		got := ResolveLDAPURL("ldap://localhost:389")
		if got != "ldap://localhost:389" {
			t.Fatalf("ResolveLDAPURL = %q, want unchanged outside Docker", got)
		}
	}
}
