package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/openapi"
	"gopkg.in/yaml.v3"
)

var forbiddenPathPrefixes = []string{
	"/admin/",
	"/settings/system",
	"/settings/buckets",
	"/users",
	"/gateway",
	"/tenants",
	"/webhooks",
	"/activity",
	"/federation",
	"/cluster",
	"/settings/ldap",
	"/auth/",
	"/mfa/",
	"/sts/",
}

var requiredCommunityPaths = []string{
	"/health",
	"/me",
	"/buckets",
	"/buckets/{bucket}/objects",
	"/keys",
	"/presign",
	"/usage",
	"/tokens",
	"/buckets/{bucket}/shares",
	"/public/share/{token}",
}

func TestOpenAPISpecEndpoint(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi.json status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("content-type %q", ct)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if doc["openapi"] != "3.1.0" {
		t.Fatalf("openapi version %v", doc["openapi"])
	}
	info, _ := doc["info"].(map[string]any)
	if title, _ := info["title"].(string); !strings.Contains(title, "Community") {
		t.Fatalf("expected Community title, got %q", title)
	}
	paths, _ := doc["paths"].(map[string]any)
	if paths == nil {
		t.Fatal("missing paths")
	}
	for _, p := range requiredCommunityPaths {
		if _, ok := paths[p]; !ok {
			t.Errorf("community path missing in spec: %s", p)
		}
	}
}

func TestOpenAPINoAdminPaths(t *testing.T) {
	paths := openAPISpecPaths(t)
	var found []string
	for p := range paths {
		for _, prefix := range forbiddenPathPrefixes {
			if strings.HasPrefix(p, prefix) || p == strings.TrimSuffix(prefix, "/") {
				found = append(found, p)
				break
			}
		}
		if p == "/buckets/{bucket}/policy" {
			found = append(found, p)
		}
	}
	if len(found) > 0 {
		t.Fatalf("admin paths must not appear in community spec: %s", strings.Join(found, ", "))
	}
}

func TestOpenAPIYAMLEndpoint(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi.yaml status %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "yaml") {
		t.Fatalf("unexpected content-type %q", rec.Header().Get("Content-Type"))
	}
}

func TestOpenAPISwaggerUI(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("docs status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"swagger-ui", "/api/v1/docs/assets/swagger-ui-bundle.js", "/api/v1/docs/assets/swagger-ui-init.js"} {
		if !strings.Contains(body, want) {
			t.Fatalf("swagger ui html missing %q", want)
		}
	}

	reqInit := httptest.NewRequest(http.MethodGet, "/api/v1/docs/assets/swagger-ui-init.js", nil)
	recInit := httptest.NewRecorder()
	s.Handler().ServeHTTP(recInit, reqInit)
	if recInit.Code != http.StatusOK {
		t.Fatalf("swagger init js status %d", recInit.Code)
	}
	initBody := recInit.Body.String()
	for _, want := range []string{"/api/v1/openapi.json", "persistAuthorization", "SwaggerUIBundle"} {
		if !strings.Contains(initBody, want) {
			t.Fatalf("swagger init js missing %q", want)
		}
	}
}

func TestOpenAPIDriftFromServerRoutes(t *testing.T) {
	t.Helper()
	root := findRepoRoot(t)
	serverGo := filepath.Join(root, "internal", "api", "server.go")
	body, err := os.ReadFile(serverGo)
	if err != nil {
		t.Fatal(err)
	}
	routes := parseServerRoutes(string(body))
	specPaths := openAPISpecPaths(t)

	// Every path in the community spec must exist on the server.
	var orphan []string
	for path := range specPaths {
		if !communityPathExistsOnServer(path, routes) {
			orphan = append(orphan, path)
		}
	}
	if len(orphan) > 0 {
		t.Errorf("OpenAPI spec has %d paths not registered in server.go:\n%s", len(orphan), strings.Join(orphan, "\n"))
	}
}

func communityPathExistsOnServer(openAPIPath string, routes map[string]bool) bool {
	for route := range routes {
		if routeToOpenAPIPath(route) == openAPIPath {
			return true
		}
	}
	return false
}

func TestOpenAPIEmbeddedMatchesDocs(t *testing.T) {
	root := findRepoRoot(t)
	docs := filepath.Join(root, "docs", "api", "openapi.yaml")
	embed := filepath.Join(root, "internal", "openapi", "openapi.yaml")
	a, err := os.ReadFile(docs)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(embed)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("docs/api/openapi.yaml and internal/openapi/openapi.yaml differ; run: go run tools/gen-openapi-yaml.go")
	}
	_ = openapi.SpecJSON() // ensure package init succeeds
}

var routeRe = regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) (/api/v1/[^"]+)"`)

func parseServerRoutes(src string) map[string]bool {
	out := make(map[string]bool)
	for _, m := range routeRe.FindAllStringSubmatch(src, -1) {
		out[m[1]+" "+m[2]] = true
	}
	return out
}

func routeToOpenAPIPath(route string) string {
	parts := strings.SplitN(route, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	p := parts[1]
	if !strings.HasPrefix(p, "/api/v1/") {
		return ""
	}
	p = strings.TrimPrefix(p, "/api/v1")
	p = regexp.MustCompile(`\{key\.\.\.\}`).ReplaceAllString(p, "{key}")
	return p
}

func openAPISpecPaths(t *testing.T) map[string]bool {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(openapi.SpecJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	out := make(map[string]bool, len(paths))
	for p := range paths {
		out[p] = true
	}
	return out
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func countOperations(t *testing.T) int {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(openapi.SpecJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	n := 0
	for _, methods := range paths {
		m, _ := methods.(map[string]any)
		for method := range m {
			switch method {
			case "get", "post", "put", "delete", "patch", "head", "options":
				n++
			}
		}
	}
	return n
}

func TestOpenAPIOperationCount(t *testing.T) {
	n := countOperations(t)
	if n < 45 || n > 65 {
		t.Fatalf("expected ~55 community operations, got %d", n)
	}
}

func TestOpenAPIFullTierAP0P1(t *testing.T) {
	root := findRepoRoot(t)
	full := filepath.Join(root, "docs", "api", "openapi-full.yaml")
	body, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse openapi-full: %v", err)
	}
	paths, _ := doc["paths"].(map[string]any)
	if paths == nil {
		t.Fatal("missing paths in openapi-full.yaml")
	}
	serverGo, err := os.ReadFile(filepath.Join(root, "internal", "api", "server.go"))
	if err != nil {
		t.Fatal(err)
	}
	routes := parseServerRoutes(string(serverGo))
	for _, openAPIPath := range tierAP0P1Paths() {
		if _, ok := paths[openAPIPath]; !ok {
			t.Errorf("openapi-full missing P0/P1 path: %s", openAPIPath)
		}
		if !communityPathExistsOnServer(openAPIPath, routes) {
			t.Errorf("server.go missing P0/P1 route for: %s", openAPIPath)
		}
	}
}

func tierAP0P1Paths() []string {
	return []string{
		"/health", "/me", "/me/password", "/me/mfa/webauthn/register/begin", "/me/mfa/webauthn/register/finish",
		"/admin/login", "/admin/logout", "/mfa/login", "/buckets", "/buckets/{bucket}/objects",
		"/buckets/{bucket}/shares", "/keys", "/presign", "/usage", "/tokens",
		"/public/share/{token}", "/webhooks", "/hooks/test", "/federation/clusters",
		"/settings/system", "/users", "/activity", "/trash",
	}
}

func TestOpenAPISecurityScheme(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(openapi.SpecJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	components, _ := doc["components"].(map[string]any)
	schemes, _ := components["securitySchemes"].(map[string]any)
	if _, ok := schemes["BearerAPIToken"]; !ok {
		t.Fatal("missing BearerAPIToken security scheme")
	}
	if _, ok := schemes["BearerJWT"]; ok {
		t.Fatal("BearerJWT must not be in community spec")
	}
}

