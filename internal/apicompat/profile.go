// Package apicompat is the server-capability detector Desktop uses.
// A0 keeps it on the server side so the contract is tested before the agent exists.
package apicompat

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	ProfileCapabilities = "capabilities"
	Profile160          = "1.6.0"
	Profile150          = "1.5.0"
	ProfileBasic        = "basic"
	ProfileRejected     = "rejected"
)

// Result is what the agent knows after probing a server.
type Result struct {
	Profile              string
	ServerVersion        string
	SpecVersion          string
	LoginMethods         []string
	Features             map[string]int
	NeedsAuthForFeatures bool
	MinSupported         string
	MinRequired          string
	MinRequiredReason    string
	AdvisoryURL          string
	MaxTested            string
	RejectReason         string
	Warning              string
	DeprecationsPresent  bool
}

// DetectAnonymous applies the unauthenticated half of the probe.
// A 200 capabilities document without features means the version is known
// and the client must authenticate before treating features as off.
func DetectAnonymous(capStatus int, capBody []byte, healthStatus int, healthBody []byte) Result {
	if capStatus == 200 {
		if res, ok := fromCapabilities(capBody, false); ok {
			return res
		}
	}
	return fromHealthz(healthStatus, healthBody)
}

// ApplyAuthenticated merges an authenticated capabilities document.
// Missing feature keys are off. A document that still has no features object
// leaves the previous feature set in place.
func ApplyAuthenticated(prev Result, capStatus int, capBody []byte) Result {
	if capStatus != 200 {
		return prev
	}
	res, ok := fromCapabilities(capBody, true)
	if !ok {
		return prev
	}
	if !res.NeedsAuthForFeatures {
		prev.Features = res.Features
	}
	prev.Profile = ProfileCapabilities
	prev.ServerVersion = res.ServerVersion
	prev.SpecVersion = res.SpecVersion
	if len(res.LoginMethods) > 0 {
		prev.LoginMethods = res.LoginMethods
	}
	prev.MinSupported = res.MinSupported
	prev.MinRequired = res.MinRequired
	prev.MinRequiredReason = res.MinRequiredReason
	prev.AdvisoryURL = res.AdvisoryURL
	prev.MaxTested = res.MaxTested
	prev.DeprecationsPresent = res.DeprecationsPresent
	prev.NeedsAuthForFeatures = false
	return prev
}

func fromCapabilities(body []byte, authenticated bool) (Result, bool) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return Result{}, false
	}
	var api string
	if err := json.Unmarshal(raw["api"], &api); err != nil || api != "v1" {
		return Result{}, false
	}
	res := Result{Profile: ProfileCapabilities, Features: map[string]int{}}
	_ = json.Unmarshal(raw["server_version"], &res.ServerVersion)
	_ = json.Unmarshal(raw["spec_version"], &res.SpecVersion)
	_ = json.Unmarshal(raw["login_methods"], &res.LoginMethods)
	featRaw, hasFeatures := raw["features"]
	if !hasFeatures || string(featRaw) == "null" {
		res.NeedsAuthForFeatures = !authenticated
		res.Features = nil
		return res, true
	}
	var features map[string]int
	if err := json.Unmarshal(featRaw, &features); err != nil {
		return Result{}, false
	}
	res.Features = features
	res.NeedsAuthForFeatures = false
	if clients, ok := raw["clients"]; ok && string(clients) != "null" {
		var doc struct {
			Desktop struct {
				MinSupported      string `json:"min_supported"`
				MinRequired       string `json:"min_required"`
				MinRequiredReason string `json:"min_required_reason"`
				AdvisoryURL       string `json:"advisory_url"`
				MaxTested         string `json:"max_tested"`
			} `json:"desktop"`
		}
		_ = json.Unmarshal(clients, &doc)
		res.MinSupported = doc.Desktop.MinSupported
		res.MinRequired = doc.Desktop.MinRequired
		res.MinRequiredReason = doc.Desktop.MinRequiredReason
		res.AdvisoryURL = doc.Desktop.AdvisoryURL
		res.MaxTested = doc.Desktop.MaxTested
	}
	if dep, ok := raw["deprecations"]; ok && string(dep) != "null" && string(dep) != "[]" {
		res.DeprecationsPresent = true
	}
	return res, true
}

func fromHealthz(status int, body []byte) Result {
	if status != 200 {
		return Result{
			Profile:  ProfileBasic,
			Features: features150(),
			Warning:  "server version is unknown",
		}
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return Result{Profile: ProfileBasic, Features: features150(), Warning: "server version is unknown"}
	}
	verRaw, hasVersion := doc["version"]
	var version string
	if hasVersion {
		if err := json.Unmarshal(verRaw, &version); err != nil {
			return Result{Profile: ProfileBasic, Features: features150(), Warning: "server version is unknown"}
		}
	}
	_, hasKMS := doc["kms"]
	version = strings.TrimSpace(version)
	if semverAtLeast(version, 1, 6, 0) || hasKMS {
		return Result{Profile: Profile160, ServerVersion: version, Features: features160()}
	}
	if strings.EqualFold(version, "dev") {
		return Result{Profile: Profile150, ServerVersion: version, Features: features150()}
	}
	if isSemver(version) && semverLess(version, [3]int{1, 5, 0}) {
		return Result{Profile: ProfileRejected, ServerVersion: version, RejectReason: "server version is not supported, 1.5.0 or newer is required"}
	}
	if isSemver(version) && versionMajorMinor(version) == "1.5" {
		return Result{Profile: Profile150, ServerVersion: version, Features: features150()}
	}
	return Result{Profile: ProfileBasic, ServerVersion: version, Features: features150(), Warning: "server version is unknown"}
}

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
	f := features150()
	f["sharing.policy"] = 1
	f["sharing.revoke_all"] = 1
	f["auth.step_up"] = 1
	f["encryption.kms"] = 1
	return f
}

// Disposition is how Desktop reacts to clients.desktop.
type Disposition string

const (
	DispositionOK       Disposition = "ok"
	DispositionWarn     Disposition = "warn"
	DispositionBlock    Disposition = "block"
	DispositionUntested Disposition = "untested"
)

// DesktopDisposition implements the client-version rules.
// A min_required without reason security is only a warning.
func DesktopDisposition(appVersion, minSupported, minRequired, minRequiredReason, maxTested string) Disposition {
	if minRequired != "" && semverLess(appVersion, parseSemver(minRequired)) && minRequiredReason == "security" {
		return DispositionBlock
	}
	if minSupported != "" && semverLess(appVersion, parseSemver(minSupported)) {
		return DispositionWarn
	}
	if maxTested != "" && semverLess(maxTested, parseSemver(appVersion)) {
		return DispositionUntested
	}
	return DispositionOK
}

func isSemver(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return false
	}
	for i := 0; i < len(parts) && i < 3; i++ {
		if _, err := strconv.Atoi(parts[i]); err != nil {
			return false
		}
	}
	return true
}

func semverAtLeast(v string, major, minor, patch int) bool {
	if !isSemver(v) {
		return false
	}
	nums := parseSemver(v)
	if nums[0] != major {
		return nums[0] > major
	}
	if nums[1] != minor {
		return nums[1] > minor
	}
	return nums[2] >= patch
}

func semverLess(v string, than [3]int) bool {
	if !isSemver(v) {
		return false
	}
	nums := parseSemver(v)
	if nums[0] != than[0] {
		return nums[0] < than[0]
	}
	if nums[1] != than[1] {
		return nums[1] < than[1]
	}
	return nums[2] < than[2]
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	var nums [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		nums[i], _ = strconv.Atoi(parts[i])
	}
	return nums
}

func versionMajorMinor(v string) string {
	nums := parseSemver(v)
	return strconv.Itoa(nums[0]) + "." + strconv.Itoa(nums[1])
}
