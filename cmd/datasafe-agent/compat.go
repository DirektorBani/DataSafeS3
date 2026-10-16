package main

import (
	"fmt"
	"strconv"
	"strings"
)

// DesktopVersion is the Desktop 1.0 scaffold compared with clients.desktop.
const DesktopVersion = "1.0.0-dev"

// Feature columns from spec 4.6.6. A test checks these maps.
func features150() map[string]int {
	return map[string]int{
		"auth.password":     1,
		"auth.totp":         1,
		"auth.oidc":         1,
		"objects.multipart": 1,
		"sharing.links":     1,
	}
}

func features160() map[string]int {
	out := features150()
	out["sharing.policy"] = 1
	out["sharing.revoke_all"] = 1
	out["auth.step_up"] = 1
	out["encryption.kms"] = 1
	return out
}

// ServerCard is the non-secret result of version detection.
type ServerCard struct {
	Profile       string         `json:"profile"`
	Features      map[string]int `json:"features"`
	LoginMethods  []string       `json:"login_methods,omitempty"`
	ServerVersion string         `json:"server_version,omitempty"`
	SpecVersion   string         `json:"spec_version,omitempty"`
	Warning       string         `json:"warning,omitempty"`
	Deprecation   string         `json:"deprecation,omitempty"`
	Blocked       bool           `json:"blocked"`
	AdvisoryURL   string         `json:"advisory_url,omitempty"`
	Rejected      bool           `json:"rejected"`
	BrowseAllowed bool           `json:"browse_allowed"`
	Reauth        bool           `json:"reauth"`
	MFASetup      bool           `json:"mfa_setup"`
	ExpiresSoon   bool           `json:"expires_soon"`
	ClientVersion string         `json:"client_version"`
}

type healthInfo struct {
	Version string
	HasKMS  bool
	Reached bool
}

type capsInfo struct {
	Status        int
	API           string
	HasFeatures   bool
	Features      map[string]int
	LoginMethods  []string
	ServerVersion string
	SpecVersion   string
	Desktop       map[string]any
	Deprecations  []map[string]any
	Reached       bool
}

// detect applies spec 4.6.5. A public capabilities body without features
// (the accepted 1.6.1 shape) falls through to /healthz. An authenticated
// body with features replaces that column.
func detect(public, authed *capsInfo, health healthInfo, meReached bool, clientVersion string) ServerCard {
	if clientVersion == "" {
		clientVersion = DesktopVersion
	}
	card := ServerCard{
		Features:      map[string]int{},
		ClientVersion: clientVersion,
		BrowseAllowed: true,
	}
	chosen := (*capsInfo)(nil)
	if authed != nil && authed.Reached && authed.Status == 200 && authed.API == "v1" && authed.HasFeatures {
		chosen = authed
	} else if public != nil && public.Reached && public.Status == 200 && public.API == "v1" && public.HasFeatures {
		chosen = public
	}
	if chosen != nil {
		card.Profile = "features"
		card.Features = cloneFeatures(chosen.Features)
		card.LoginMethods = append([]string(nil), chosen.LoginMethods...)
		card.ServerVersion = chosen.ServerVersion
		card.SpecVersion = chosen.SpecVersion
		applyDesktopPolicy(&card, chosen.Desktop, clientVersion)
		card.Deprecation = deprecationText(chosen.Deprecations)
	} else {
		if public != nil && public.Reached && public.Status == 200 {
			card.LoginMethods = append([]string(nil), public.LoginMethods...)
			card.ServerVersion = public.ServerVersion
			card.SpecVersion = public.SpecVersion
		}
		classifyHealth(&card, health, meReached)
	}
	if card.Rejected || card.Blocked {
		card.BrowseAllowed = false
	}
	if card.Warning == "" && card.Profile == "basic" {
		card.Warning = "server version is unknown"
	}
	return card
}

func classifyHealth(card *ServerCard, health healthInfo, meReached bool) {
	if !health.Reached {
		if meReached {
			card.Profile = "basic"
			card.Features = features150()
			return
		}
		card.Profile = "unreachable"
		card.Rejected = true
		card.Warning = "server is unreachable"
		return
	}
	version := strings.TrimSpace(health.Version)
	card.ServerVersion = version
	switch {
	case version == "" || (isSemver(version) && versionLT(version, "1.5.0")):
		// v1.5.0+ always sends a non-empty version (tag v1.5.0,
		// TestHealthReportsVersionAndCommit). A reachable /healthz with
		// the field missing or blank is older than 1.5.0. v1.4.1's
		// handler does not write the field at all.
		card.Profile = "rejected"
		card.Rejected = true
		card.Warning = "server is older than 1.5.0"
	case versionGE(version, "1.6.0") || health.HasKMS:
		// 1.6.0 and newer, including a 1.6.x build stamped 1.6.0 and a dev
		// build that already advertises the kms key.
		card.Profile = "1.6.0"
		card.Features = features160()
	case strings.EqualFold(version, "dev"):
		card.Profile = "1.5.0"
		card.Features = features150()
	case isSemver(version) && versionLT(version, "1.6.0"):
		card.Profile = "1.5.0"
		card.Features = features150()
	default:
		card.Profile = "basic"
		card.Features = features150()
	}
}

func applyDesktopPolicy(card *ServerCard, desktop map[string]any, clientVersion string) {
	if desktop == nil {
		return
	}
	minSupported, _ := desktop["min_supported"].(string)
	minRequired, _ := desktop["min_required"].(string)
	reason, _ := desktop["min_required_reason"].(string)
	advisory, _ := desktop["advisory_url"].(string)
	if minRequired != "" && below(clientVersion, minRequired) {
		if strings.EqualFold(strings.TrimSpace(reason), "security") {
			card.Blocked = true
			card.AdvisoryURL = advisory
			card.Warning = "desktop version is blocked by the server"
			return
		}
		card.Warning = "desktop version is older than the server requires"
	}
	if card.Warning == "" && minSupported != "" && below(clientVersion, minSupported) {
		card.Warning = "desktop version is older than the server supports"
	}
}

func deprecationText(items []map[string]any) string {
	if len(items) == 0 {
		return ""
	}
	first := items[0]
	method, _ := first["method"].(string)
	path, _ := first["path"].(string)
	if method == "" || path == "" {
		return "server marked an operation deprecated"
	}
	return fmt.Sprintf("server marked %s %s deprecated", method, path)
}

func cloneFeatures(in map[string]int) map[string]int {
	out := make(map[string]int, len(in))
	for k, v := range in {
		if k == "" || v < 1 {
			continue
		}
		out[k] = v
	}
	return out
}

type semver struct {
	major, minor, patch int
	pre                 string
	ok                  bool
}

func parseSemver(v string) semver {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" || strings.EqualFold(v, "dev") {
		return semver{}
	}
	pre := ""
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		pre = v[i:]
		if strings.HasPrefix(pre, "+") {
			pre = ""
		}
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return semver{}
	}
	nums := [3]int{}
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			continue
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return semver{}
		}
		nums[i] = n
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2], pre: pre, ok: true}
}

func isSemver(v string) bool { return parseSemver(v).ok }

func versionLT(a, b string) bool {
	pa, pb := parseSemver(a), parseSemver(b)
	if !pa.ok || !pb.ok {
		return false
	}
	if pa.major != pb.major {
		return pa.major < pb.major
	}
	if pa.minor != pb.minor {
		return pa.minor < pb.minor
	}
	if pa.patch != pb.patch {
		return pa.patch < pb.patch
	}
	if pa.pre == pb.pre {
		return false
	}
	if pa.pre == "" {
		return false
	}
	if pb.pre == "" {
		return true
	}
	return pa.pre < pb.pre
}

func versionGE(a, b string) bool {
	return isSemver(a) && !versionLT(a, b)
}

// below reports whether the desktop build is older than floor.
// A -dev (or any prerelease) of the same X.Y.Z is not older than X.Y.Z,
// so the 1.0.0-dev scaffold is not warned against min_supported 1.0.0.
func below(client, floor string) bool {
	pc, pf := parseSemver(client), parseSemver(floor)
	if !pc.ok || !pf.ok {
		return false
	}
	if pc.major == pf.major && pc.minor == pf.minor && pc.patch == pf.patch {
		return false
	}
	return versionLT(client, floor)
}
