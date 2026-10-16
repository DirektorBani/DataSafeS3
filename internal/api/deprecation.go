package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/DirektorBani/datasafe/internal/openapi"
	"gopkg.in/yaml.v3"
)

// DeprecationNotice is one deprecated operation. Production notices come from
// the embedded OpenAPI document. A0 ships none; tests load a fixture.
type DeprecationNotice struct {
	Method       string
	Path         string
	Field        string
	Since        string
	Sunset       time.Time
	Replacement  string
	NotesURL     string
	DeprecatedAt int64
}

func init() {
	notices, err := parseDeprecations(openapi.SpecYAML())
	if err != nil {
		panic("api: parse openapi deprecations: " + err.Error())
	}
	apiDeprecations = notices
}

var apiDeprecations []DeprecationNotice

func applyDeprecationHeaders(w http.ResponseWriter, r *http.Request) {
	if n, ok := matchDeprecation(r.Method, r.URL.Path, apiDeprecations); ok {
		writeDeprecationHeaders(w, n)
	}
}

func writeDeprecationHeaders(w http.ResponseWriter, n DeprecationNotice) {
	if n.DeprecatedAt > 0 {
		w.Header().Set("Deprecation", "@"+strconv.FormatInt(n.DeprecatedAt, 10))
	}
	if !n.Sunset.IsZero() {
		w.Header().Set("Sunset", n.Sunset.UTC().Format(http.TimeFormat))
	}
	if n.NotesURL != "" {
		w.Header().Set("Link", "<"+n.NotesURL+`>; rel="deprecation"`)
	}
}

func matchDeprecation(method, urlPath string, notices []DeprecationNotice) (DeprecationNotice, bool) {
	method = strings.ToLower(method)
	path := urlPath
	if strings.HasPrefix(path, "/api/v1/") {
		path = strings.TrimPrefix(path, "/api/v1")
	} else if path == "/api/v1" {
		path = "/"
	}
	for _, n := range notices {
		if strings.ToLower(n.Method) != method {
			continue
		}
		if pathPatternMatch(n.Path, path) {
			return n, true
		}
	}
	return DeprecationNotice{}, false
}

func pathPatternMatch(pattern, path string) bool {
	if pattern == path {
		return true
	}
	pp := strings.Split(strings.Trim(pattern, "/"), "/")
	rp := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i < len(pp); i++ {
		seg := pp[i]
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") && strings.Contains(seg, "...") {
			return true
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if i >= len(rp) || rp[i] == "" {
				return false
			}
			continue
		}
		if i >= len(rp) || rp[i] != seg {
			return false
		}
	}
	return len(pp) == len(rp)
}

func parseDeprecations(spec []byte) ([]DeprecationNotice, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(spec, &doc); err != nil {
		return nil, err
	}
	paths, _ := doc["paths"].(map[string]any)
	var out []DeprecationNotice
	for path, item := range paths {
		ops, _ := item.(map[string]any)
		for method, raw := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete", "head":
			default:
				continue
			}
			op, _ := raw.(map[string]any)
			if op == nil || !yamlBool(op["deprecated"]) {
				continue
			}
			n := DeprecationNotice{
				Method:      method,
				Path:        path,
				Since:       yamlString(op["x-since"]),
				Replacement: yamlString(op["x-replacement"]),
				NotesURL:    yamlString(op["x-notes-url"]),
				Field:       yamlString(op["x-deprecated-field"]),
			}
			if at := yamlString(op["x-deprecated-at"]); at != "" {
				if n64, err := strconv.ParseInt(at, 10, 64); err == nil {
					n.DeprecatedAt = n64
				}
			}
			if sunset := yamlString(op["x-sunset"]); sunset != "" {
				if ts, err := http.ParseTime(sunset); err == nil {
					n.Sunset = ts
				} else if d, err := time.Parse("2006-01-02", sunset); err == nil {
					n.Sunset = d.UTC()
				}
			}
			out = append(out, n)
		}
	}
	return out, nil
}

func deprecationPublicPath(path string) string {
	if path == "/healthz" || strings.HasPrefix(path, "/healthz/") {
		return path
	}
	if strings.HasPrefix(path, "/api/") {
		return path
	}
	return "/api/v1" + path
}

func yamlBool(v any) bool {
	b, ok := v.(bool)
	return ok && b
}

func yamlString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	default:
		return ""
	}
}

// deprecationsForCapabilities returns the public deprecation records.
func deprecationsForCapabilities() []map[string]any {
	if len(apiDeprecations) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(apiDeprecations))
	for _, n := range apiDeprecations {
		item := map[string]any{
			"method":      strings.ToUpper(n.Method),
			"path":        deprecationPublicPath(n.Path),
			"since":       n.Since,
			"replacement": n.Replacement,
			"notes_url":   n.NotesURL,
		}
		if n.Field != "" {
			item["field"] = n.Field
		}
		if !n.Sunset.IsZero() {
			item["sunset"] = n.Sunset.UTC().Format(time.RFC3339)
		} else {
			item["sunset"] = nil
		}
		out = append(out, item)
	}
	return out
}
