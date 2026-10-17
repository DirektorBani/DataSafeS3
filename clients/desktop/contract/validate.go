package contract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Envelope is one recorded request/response pair. Secrets in body are
// already replaced with <masked>.
type Envelope struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// CheckFile validates one fixture against the shapes in v1.yaml.
// A legacy error body is the `error` string. A plain-text 404 is
// `body.raw`. `code`, `message`, and `request_id` are required only when
// errors.codes is on, which 1.5.0 and 1.6.0 do not advertise.
func CheckFile(specPath, fixturePath string) error {
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		return err
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("%s: %w", fixturePath, err)
	}
	return CheckEnvelope(specPath, env)
}

// CheckEnvelope reports a missing required field or a wrong JSON type.
func CheckEnvelope(specPath string, env Envelope) error {
	if env.Method == "" || env.Path == "" || env.Status == 0 {
		return fmt.Errorf("fixture needs method, path, and status")
	}
	var body any
	if err := json.Unmarshal(env.Body, &body); err != nil {
		return fmt.Errorf("%s %s: body: %w", env.Method, env.Path, err)
	}
	obj, ok := body.(map[string]any)
	if !ok {
		return fmt.Errorf("%s %s: body must be an object", env.Method, env.Path)
	}
	switch {
	case env.Path == "/healthz" && env.Status == 200:
		required, err := schemaRequired(specPath, []string{"paths", "/healthz", "get", "responses", "200", "content", "application/json", "schema"})
		if err != nil {
			return err
		}
		return requireFields(env.Path, obj, required)
	case env.Path == "/api/v1/capabilities" && env.Status == 200:
		required, err := componentRequired(specPath, "Capabilities")
		if err != nil {
			return err
		}
		if err := requireFields(env.Path, obj, required); err != nil {
			return err
		}
		if api, _ := obj["api"].(string); api != "v1" {
			return fmt.Errorf("%s: api must be v1", env.Path)
		}
		return nil
	case strings.HasSuffix(env.Path, "/objects") && env.Status == 200:
		if objects, exists := obj["objects"]; exists && !isArray(objects) {
			return fmt.Errorf("%s: objects must be an array", env.Path)
		}
		if folders, exists := obj["folders"]; exists && !isArray(folders) {
			return fmt.Errorf("%s: folders must be an array", env.Path)
		}
		return nil
	case env.Status >= 400:
		// A legacy server sends {"error":"..."}. Go's default mux answers
		// an unknown route with text/plain "404 page not found", which
		// record.sh stores as body.raw.
		if raw, ok := obj["raw"].(string); ok && strings.TrimSpace(raw) != "" {
			return nil
		}
		if _, ok := obj["error"].(string); !ok {
			return fmt.Errorf("%s: error status needs a string error field or a raw body", env.Path)
		}
		return nil
	default:
		return nil
	}
}

func isArray(v any) bool {
	_, ok := v.([]any)
	return ok
}

func requireFields(path string, obj map[string]any, required []string) error {
	var missing []string
	for _, key := range required {
		if _, ok := obj[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: missing %s", path, strings.Join(missing, ", "))
	}
	return nil
}

func componentRequired(specPath, name string) ([]string, error) {
	return schemaRequired(specPath, []string{"components", "schemas", name})
}

func schemaRequired(specPath string, path []string) ([]string, error) {
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	node := any(doc)
	for _, key := range path {
		next, ok := lookup(node, key)
		if !ok {
			return nil, fmt.Errorf("openapi path %s is missing", strings.Join(path, "."))
		}
		node = next
	}
	obj, ok := node.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("openapi path %s is not an object", strings.Join(path, "."))
	}
	req, ok := obj["required"]
	if !ok {
		return nil, fmt.Errorf("openapi path %s has no required list", strings.Join(path, "."))
	}
	items, ok := req.([]any)
	if !ok {
		return nil, fmt.Errorf("openapi required list is not an array")
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("openapi required entry is not a string")
		}
		out = append(out, text)
	}
	return out, nil
}

func lookup(node any, key string) (any, bool) {
	obj, ok := node.(map[string]any)
	if !ok {
		return nil, false
	}
	value, ok := obj[key]
	return value, ok
}

// WalkFixtures calls fn for every .json file under dir.
func WalkFixtures(dir string, fn func(path string) error) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		return fn(path)
	})
}
