package main

import "strings"

// desktopCredentialSuffix is the Windows Credential Manager target suffix
// the keyring crate builds: "<profile_id>/<kind>.datasafe-desktop".
const desktopCredentialSuffix = ".datasafe-desktop"

// isDesktopCredential reports whether target is one of our Credential
// Manager names. The check is the target string itself. It does not read
// cmdkey output, whose labels depend on the UI language (English "Target:",
// Russian "Конечный файл:").
func isDesktopCredential(target string) bool {
	if target == "" || strings.ContainsAny(target, " \t\r\n:=") {
		return false
	}
	if len(target) <= len(desktopCredentialSuffix)+2 {
		return false
	}
	if !strings.EqualFold(target[len(target)-len(desktopCredentialSuffix):], desktopCredentialSuffix) {
		return false
	}
	head := target[:len(target)-len(desktopCredentialSuffix)]
	slash := strings.IndexByte(head, '/')
	if slash <= 0 || slash != strings.LastIndexByte(head, '/') || slash >= len(head)-1 {
		return false
	}
	return true
}

// selectDesktopTargets keeps credential target names this app created.
// Display lines from cmdkey, in any language, are not target names and are
// left out.
func selectDesktopTargets(names []string) []string {
	out := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !isDesktopCredential(name) {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}
