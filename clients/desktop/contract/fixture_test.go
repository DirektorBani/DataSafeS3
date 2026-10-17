package contract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestMatrixCoversTheTable(t *testing.T) {
	if len(Matrix) != 15 {
		t.Fatalf("table 4.6.6 has 15 feature keys, got %d", len(Matrix))
	}
	v15 := Column("1.5.0")
	v16 := Column("1.6.0")
	for _, name := range []string{"auth.password", "auth.totp", "auth.oidc", "objects.multipart", "sharing.links"} {
		if v15[name] != 1 || v16[name] != 1 {
			t.Fatalf("%s must be on for 1.5.0 and 1.6.0", name)
		}
	}
	for _, name := range []string{"sharing.policy", "sharing.revoke_all", "auth.step_up", "encryption.kms"} {
		if _, ok := v15[name]; ok || v16[name] != 1 {
			t.Fatalf("%s is 1.6.0 only", name)
		}
	}
	for _, name := range []string{"auth.refresh", "objects.list_parts", "objects.range", "errors.codes", "server.secrets_stdin", "server.schema_version"} {
		if _, ok := v15[name]; ok {
			t.Fatalf("1.5.0 includes %s", name)
		}
		if _, ok := v16[name]; ok {
			t.Fatalf("1.6.0 includes %s", name)
		}
	}
}

func TestFixturesMatchSpec(t *testing.T) {
	root := repoRoot(t)
	spec := filepath.Join(root, "api", "openapi", "v1.yaml")
	dir := filepath.Join(root, "clients", "desktop", "contract", "fixtures")
	broken := filepath.Join(dir, "partial", "capabilities-broken.json")
	renamed := filepath.Join(dir, "broken", "capabilities-renamed.json")
	n := 0
	err := WalkFixtures(dir, func(path string) error {
		if path == broken || path == renamed {
			return nil
		}
		n++
		if err := CheckFile(spec, path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 10 {
		t.Fatalf("expected the 1.5.0 and 1.6.0 fixtures, checked %d", n)
	}
}

func TestRenamedFieldFailsContract(t *testing.T) {
	root := repoRoot(t)
	spec := filepath.Join(root, "api", "openapi", "v1.yaml")
	path := filepath.Join(root, "clients", "desktop", "contract", "fixtures", "broken", "capabilities-renamed.json")
	err := CheckFile(spec, path)
	if err == nil {
		t.Fatal("renaming api to apiVersion must fail the contract check")
	}
	if !strings.Contains(err.Error(), "api") {
		t.Fatalf("error should name the missing field, got %v", err)
	}
}

func TestBrokenCapabilitiesAreNotJSON(t *testing.T) {
	root := repoRoot(t)
	raw, err := os.ReadFile(filepath.Join(root, "clients", "desktop", "contract", "fixtures", "partial", "capabilities-broken.json"))
	if err != nil {
		t.Fatal(err)
	}
	var body any
	if err := json.Unmarshal(raw, &body); err == nil {
		t.Fatal("the broken capabilities fixture must not parse")
	}
}

func TestSecretsClean(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "clients", "desktop", "contract", "fixtures")
	forbidden := []*regexp.Regexp{
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`(?i)bearer [A-Za-z0-9._-]{8,}`),
		regexp.MustCompile(`X-Amz-Signature=`),
		regexp.MustCompile(`(?i)aws4_request`),
	}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(raw)
		for _, re := range forbidden {
			if re.FindStringIndex(text) != nil {
				t.Errorf("%s matches %s", path, re.String())
			}
		}
		var body any
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil
		}
		walkSecrets(t, path, body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func walkSecrets(t *testing.T, path string, node any) {
	t.Helper()
	switch n := node.(type) {
	case map[string]any:
		for key, value := range n {
			if secretField(key) {
				text, ok := value.(string)
				if ok && text != "" && text != "<masked>" {
					t.Errorf("%s field %s is not masked", path, key)
				}
			}
			walkSecrets(t, path, value)
		}
	case []any:
		for _, item := range n {
			walkSecrets(t, path, item)
		}
	}
}

func secretField(key string) bool {
	switch strings.ToLower(key) {
	case "password", "secret", "token", "signature", "authorization", "request_id":
		return true
	default:
		return false
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(dir, "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "api", "openapi", "v1.yaml")); err != nil {
		t.Fatalf("openapi spec not found from %s", dir)
	}
	return root
}
