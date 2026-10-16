package api_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/openapi"
	"gopkg.in/yaml.v3"
)

// TestOpenAPIRouteCoverage compares mux.HandleFunc patterns in server.go with
// api/openapi/v1.yaml. A route that is not described and not listed in
// openapiRouteExceptions fails. A described operation with no route fails too.
func TestOpenAPIRouteCoverage(t *testing.T) {
	root := findRepoRoot(t)
	routes := astRoutes(t, filepath.Join(root, "internal", "api", "server.go"))
	described := describedRoutes(t)

	var missing, orphans []string
	for route := range routes {
		if route == "GET /healthz" {
			if _, ok := described[route]; !ok {
				missing = append(missing, route)
			}
			continue
		}
		if _, ok := described[route]; ok {
			if _, excused := openapiRouteExceptions[route]; excused {
				t.Errorf("route %s is described and also listed as an exception", route)
			}
			continue
		}
		if reason := openapiRouteExceptions[route]; reason == "" {
			missing = append(missing, route)
		}
	}
	for route := range described {
		if strings.HasPrefix(route, "GET /healthz") || strings.Contains(route, " /api/v1/") {
			if _, ok := routes[route]; !ok {
				orphans = append(orphans, route)
			}
		}
	}
	for route, reason := range openapiRouteExceptions {
		if _, ok := routes[route]; !ok {
			t.Errorf("exception %s is not a server route (%s)", route, reason)
		}
		if strings.TrimSpace(reason) == "" {
			t.Errorf("exception %s has no reason", route)
		}
	}
	sort.Strings(missing)
	sort.Strings(orphans)
	if len(missing) > 0 {
		t.Errorf("routes missing from the contract and from the exception list:\n%s", strings.Join(missing, "\n"))
	}
	if len(orphans) > 0 {
		t.Errorf("contract operations with no server route:\n%s", strings.Join(orphans, "\n"))
	}
}

func astRoutes(t *testing.T, path string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "HandleFunc" || len(call.Args) == 0 {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		pattern, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatal(err)
		}
		method, urlPath, ok := strings.Cut(pattern, " ")
		if !ok {
			return true
		}
		if urlPath != "/healthz" && !strings.HasPrefix(urlPath, "/api/v1/") {
			return true
		}
		urlPath = strings.ReplaceAll(urlPath, "{key...}", "{key}")
		out[method+" "+urlPath] = true
		return true
	})
	if len(out) < 50 {
		t.Fatalf("parsed %d routes from %s", len(out), path)
	}
	return out
}

func yamlUnmarshal(body []byte, dest any) error {
	return yaml.Unmarshal(body, dest)
}

func describedRoutes(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(findRepoRoot(t), "api", "openapi", "v1.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(openapi.SpecYAML()) != string(body) {
		t.Fatal("embedded spec is not the canonical file")
	}
	var doc map[string]any
	if err := yamlUnmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	out := map[string]bool{}
	for path, item := range paths {
		ops, _ := item.(map[string]any)
		full := path
		if path != "/healthz" {
			full = "/api/v1" + path
		}
		for method := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete", "head":
				out[strings.ToUpper(method)+" "+full] = true
			}
		}
	}
	return out
}
