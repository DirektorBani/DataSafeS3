package contract

import (
	"strconv"
	"strings"
)

// Classify applies spec 4.6.5 points 1 and 2 to already-decoded bodies.
// A 200 capabilities body with api v1 and a features object wins. A 404,
// a body without features, or a body that did not parse (hasFeatures false
// and api empty) uses /healthz.
func Classify(publicStatus int, public map[string]any, authedStatus int, authed map[string]any, healthVersion string, hasKMS bool) (string, map[string]int) {
	if features, ok := featureMap(authedStatus, authed); ok {
		return "features", features
	}
	if features, ok := featureMap(publicStatus, public); ok {
		return "features", features
	}
	version := strings.TrimSpace(healthVersion)
	switch {
	case version == "" || (semverOK(version) && versionLT(version, "1.5.0")):
		return "rejected", map[string]int{}
	case versionGE(version, "1.6.0") || hasKMS:
		return "1.6.0", Column("1.6.0")
	case strings.EqualFold(version, "dev") || (semverOK(version) && versionLT(version, "1.6.0")):
		return "1.5.0", Column("1.5.0")
	default:
		return "basic", Column("1.5.0")
	}
}

func featureMap(status int, body map[string]any) (map[string]int, bool) {
	if status != 200 || body == nil {
		return nil, false
	}
	api, _ := body["api"].(string)
	raw, exists := body["features"]
	if api != "v1" || !exists || raw == nil {
		return nil, false
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil, false
	}
	out := map[string]int{}
	for key, value := range obj {
		switch n := value.(type) {
		case float64:
			if n >= 1 {
				out[key] = int(n)
			}
		case int:
			if n >= 1 {
				out[key] = n
			}
		}
	}
	return out, true
}

func semverOK(v string) bool {
	_, _, _, ok := parseSemver(v)
	return ok
}

func versionLT(a, b string) bool {
	am, an, ap, aok := parseSemver(a)
	bm, bn, bp, bok := parseSemver(b)
	if !aok || !bok {
		return false
	}
	if am != bm {
		return am < bm
	}
	if an != bn {
		return an < bn
	}
	return ap < bp
}

func versionGE(a, b string) bool {
	return semverOK(a) && !versionLT(a, b)
}

func parseSemver(v string) (int, int, int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" || strings.EqualFold(v, "dev") {
		return 0, 0, 0, false
	}
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	nums := [3]int{}
	for i := 0; i < len(parts); i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], true
}
