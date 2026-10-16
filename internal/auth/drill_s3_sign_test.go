package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The promote drill signs with scripts/ha/drill_s3.py. urllib adds
// Content-Type on a PUT body, and the server rejects it when unsigned.
func TestDrillS3CreateBucketSignature(t *testing.T) {
	root := moduleRoot(t)
	script := filepath.Join(root, "scripts", "ha", "drill_s3.py")
	const py = `
import json, importlib.util, sys
spec = importlib.util.spec_from_file_location("drill", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
cases = []
for method, url, body in (
    ("PUT", "http://storage-a:9000/dr-drill", b""),
    ("PUT", "http://storage-a:9000/dr-drill/obj-1", b"obj-1\n"),
    ("GET", "http://storage-a:9000/dr-drill/obj-1", b""),
):
    headers, payload = mod.sign(method, url, body, "datasafe", "datasafesecret", "us-east-1")
    cases.append({"method": method, "url": url, "headers": headers, "body": payload.decode("latin1")})
json.dump(cases, sys.stdout)
`
	cmd := exec.Command("python3", "-c", py, script)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("drill signer: %v\n%s", err, ee.Stderr)
		}
		t.Fatal(err)
	}
	var cases []struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.Unmarshal(out, &cases); err != nil {
		t.Fatal(err)
	}
	signer := NewSigner("us-east-1", func(key string) (Credentials, bool) {
		if key != "datasafe" {
			return Credentials{}, false
		}
		return Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}, true
	})
	for _, c := range cases {
		req, err := http.NewRequest(c.Method, c.URL, bytes.NewReader([]byte(c.Body)))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = req.URL.Host
		for k, v := range c.Headers {
			if strings.EqualFold(k, "Host") {
				continue
			}
			req.Header.Set(k, v)
		}
		if c.Method == http.MethodPut {
			if req.Header.Get("Content-Type") == "" {
				t.Fatalf("%s %s: Content-Type was not signed onto the request", c.Method, c.URL)
			}
			if !strings.Contains(req.Header.Get("Authorization"), "content-type") {
				t.Fatalf("%s %s: SignedHeaders omit content-type", c.Method, c.URL)
			}
		}
		if _, err := signer.Authenticate(req); err != nil {
			t.Fatalf("%s %s: %v", c.Method, c.URL, err)
		}
	}
}

func moduleRoot(t *testing.T) string {
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
