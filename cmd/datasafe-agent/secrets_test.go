package main

import (
	"reflect"
	"testing"
)

func TestDesktopCredentialSelectionIgnoresUILanguage(t *testing.T) {
	const id = "8f0c1a2b3c4d5e6f708192a3b4c5d6e7"
	real := []string{
		id + "/password.datasafe-desktop",
		id + "/token.datasafe-desktop",
		id + "/api_token.datasafe-desktop",
		id + "/access_secret.datasafe-desktop",
		id + "/expires_at.datasafe-desktop",
	}
	names := append([]string{}, real...)
	names = append(names,
		real[0],
		"other-app",
		"notes.datasafe-desktop",
		"password.datasafe-desktop",
		// cmdkey display lines. The label is localized. Neither line is a
		// credential target, and both end with the same suffix.
		"    Target: LegacyGeneric:target="+real[0],
		"    Конечный файл: LegacyGeneric:target="+real[1],
		"LegacyGeneric:target="+real[2],
	)
	got := selectDesktopTargets(names)
	if !reflect.DeepEqual(got, real) {
		t.Fatalf("got %q", got)
	}
	prefixed := legacyGenericPrefix + real[0]
	if isDesktopCredential(prefixed) {
		t.Fatal("prefixed name must not match until the prefix is removed")
	}
	if bare := bareCredentialTarget(prefixed); bare != real[0] || !isDesktopCredential(bare) {
		t.Fatalf("bare %q", bare)
	}
	if bareCredentialTarget("    Target: "+prefixed) != "    Target: "+prefixed {
		t.Fatal("a display line is not a prefixed target")
	}
}
