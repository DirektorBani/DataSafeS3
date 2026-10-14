package s3_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

// TestRealAWSAndMCChunkedPut runs aws-cli and mc when those binaries are on PATH.
// Both clients send aws-chunked bodies. The stored object must be the decoded bytes.
// TestAWSCLIDefaultChecksum runs stock aws-cli 2 transfers. Those clients
// send a CRC64NVME trailer unless checksum calculation is turned off.
// This test must not set AWS_REQUEST_CHECKSUM_CALCULATION.
func TestAWSCLIDefaultChecksum(t *testing.T) {
	if _, err := exec.LookPath("aws"); err != nil {
		t.Skip("aws is not on PATH")
	}
	srv, creds := testServer(t)
	var uploads atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Query().Has("uploads") {
			uploads.Add(1)
		}
		srv.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "e2e")

	dir := t.TempDir()
	cfg := filepath.Join(dir, "aws.config")
	if err := os.WriteFile(cfg, []byte("[default]\nregion = us-east-1\ns3 =\n    addressing_style = path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	small := []byte("hello-stock-aws")
	smallPath := filepath.Join(dir, "small.bin")
	if err := os.WriteFile(smallPath, small, 0o644); err != nil {
		t.Fatal(err)
	}
	runAWSDefault(t, ts.URL, creds, cfg, "s3", "cp", smallPath, "s3://e2e/small.bin")
	assertStored(t, ts.URL, creds, "/e2e/small.bin", small)

	before := uploads.Load()
	big := bytes.Repeat([]byte("Z"), 20<<20)
	bigPath := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(bigPath, big, 0o644); err != nil {
		t.Fatal(err)
	}
	runAWSDefault(t, ts.URL, creds, cfg, "s3", "cp", bigPath, "s3://e2e/big.bin")
	assertStored(t, ts.URL, creds, "/e2e/big.bin", big)
	if uploads.Load() == before {
		t.Fatal("20MiB s3 cp did not start a multipart upload")
	}

	syncDir := filepath.Join(dir, "syncsrc")
	if err := os.MkdirAll(syncDir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := []byte("sync-a")
	b := []byte("sync-b")
	if err := os.WriteFile(filepath.Join(syncDir, "a.txt"), a, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "b.txt"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	runAWSDefault(t, ts.URL, creds, cfg, "s3", "sync", syncDir, "s3://e2e/synced/")
	assertStored(t, ts.URL, creds, "/e2e/synced/a.txt", a)
	assertStored(t, ts.URL, creds, "/e2e/synced/b.txt", b)
}

func runAWSDefault(t *testing.T, endpoint string, creds auth.Credentials, config string, args ...string) {
	t.Helper()
	cmd := exec.Command("aws", append([]string{"--endpoint-url", endpoint}, args...)...)
	env := make([]string, 0, len(os.Environ())+6)
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AWS_REQUEST_CHECKSUM_CALCULATION=") || strings.HasPrefix(e, "AWS_RESPONSE_CHECKSUM_VALIDATION=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env,
		"AWS_ACCESS_KEY_ID="+creds.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+creds.SecretKey,
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_EC2_METADATA_DISABLED=true",
		"AWS_CONFIG_FILE="+config,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("aws %v: %v %s", args, err, out)
	}
}

func TestRealAWSAndMCChunkedPut(t *testing.T) {
	awsPath, _ := exec.LookPath("aws")
	mcPath, _ := exec.LookPath("mc")
	if awsPath == "" && mcPath == "" {
		t.Skip("aws and mc are not on PATH")
	}
	srv, creds := testServer(t)
	var uploads atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Query().Has("uploads") {
			uploads.Add(1)
		}
		srv.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "e2e")

	dir := t.TempDir()
	payload := []byte("hello-aws")
	file := filepath.Join(dir, "nine.bin")
	if err := os.WriteFile(file, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if awsPath != "" {
		t.Run("aws-single", func(t *testing.T) {
			runAWS(t, ts.URL, creds, "", "s3api", "put-object",
				"--bucket", "e2e", "--key", "aws-single.bin", "--body", file)
			assertStored(t, ts.URL, creds, "/e2e/aws-single.bin", payload)
		})
		t.Run("aws-multipart", func(t *testing.T) {
			before := uploads.Load()
			part1 := []byte("aaaa")
			part2 := []byte("bbbb")
			p1 := filepath.Join(dir, "part1.bin")
			p2 := filepath.Join(dir, "part2.bin")
			if err := os.WriteFile(p1, part1, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p2, part2, 0o644); err != nil {
				t.Fatal(err)
			}
			initOut := runAWSOut(t, ts.URL, creds, "s3api", "create-multipart-upload", "--bucket", "e2e", "--key", "aws-multi.bin")
			uploadID := jsonField(t, initOut, "UploadId")
			up1 := runAWSOut(t, ts.URL, creds, "s3api", "upload-part", "--bucket", "e2e", "--key", "aws-multi.bin", "--part-number", "1", "--upload-id", uploadID, "--body", p1)
			up2 := runAWSOut(t, ts.URL, creds, "s3api", "upload-part", "--bucket", "e2e", "--key", "aws-multi.bin", "--part-number", "2", "--upload-id", uploadID, "--body", p2)
			complete := filepath.Join(dir, "complete.json")
			doc, err := json.Marshal(map[string]any{
				"Parts": []map[string]any{
					{"ETag": jsonField(t, up1, "ETag"), "PartNumber": 1},
					{"ETag": jsonField(t, up2, "ETag"), "PartNumber": 2},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(complete, doc, 0o644); err != nil {
				t.Fatal(err)
			}
			runAWS(t, ts.URL, creds, "", "s3api", "complete-multipart-upload", "--bucket", "e2e", "--key", "aws-multi.bin", "--upload-id", uploadID, "--multipart-upload", "file://"+complete)
			assertStored(t, ts.URL, creds, "/e2e/aws-multi.bin", append(part1, part2...))
			if uploads.Load() == before {
				t.Fatal("aws upload did not create a multipart upload")
			}
		})
	}
	if mcPath != "" {
		t.Run("mc-single", func(t *testing.T) {
			mcCopy(t, mcPath, filepath.Join(dir, "mc"), ts.URL, creds, file, "ds/e2e/mc-single.bin", nil)
			assertStored(t, ts.URL, creds, "/e2e/mc-single.bin", payload)
		})
		t.Run("mc-multipart", func(t *testing.T) {
			before := uploads.Load()
			big := filepath.Join(dir, "mc-big.bin")
			body := bytes.Repeat([]byte("q"), 6<<20)
			if err := os.WriteFile(big, body, 0o644); err != nil {
				t.Fatal(err)
			}
			mcCopy(t, mcPath, filepath.Join(dir, "mc-mp"), ts.URL, creds, big, "ds/e2e/mc-multi.bin", []string{
				"MC_UPLOAD_MULTIPART_SIZE=5MiB",
			})
			assertStored(t, ts.URL, creds, "/e2e/mc-multi.bin", body)
			if uploads.Load() == before {
				t.Fatal("mc upload did not create a multipart upload")
			}
		})
	}
}

func runAWSOut(t *testing.T, endpoint string, creds auth.Credentials, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("aws", append([]string{"--endpoint-url", endpoint}, args...)...)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID="+creds.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+creds.SecretKey,
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_REQUEST_CHECKSUM_CALCULATION=when_required",
		"AWS_RESPONSE_CHECKSUM_VALIDATION=when_required",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("aws %v: %v %s", args, err, out)
	}
	return out
}

func jsonField(t *testing.T, raw []byte, name string) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("json %s: %v %s", name, err, raw)
	}
	v, _ := doc[name].(string)
	if v == "" {
		t.Fatalf("missing %s in %s", name, raw)
	}
	return v
}

func runAWS(t *testing.T, endpoint string, creds auth.Credentials, config string, args ...string) {
	t.Helper()
	cmd := exec.Command("aws", append([]string{"--endpoint-url", endpoint}, args...)...)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID="+creds.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+creds.SecretKey,
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_REQUEST_CHECKSUM_CALCULATION=when_required",
		"AWS_RESPONSE_CHECKSUM_VALIDATION=when_required",
	)
	if config != "" {
		cmd.Env = append(cmd.Env, "AWS_CONFIG_FILE="+config)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("aws %v: %v %s", args, err, out)
	}
}

func mcCopy(t *testing.T, mcPath, dir, endpoint string, creds auth.Credentials, src, dst string, extra []string) {
	t.Helper()
	alias := exec.Command(mcPath, "--config-dir", dir, "alias", "set", "ds", endpoint, creds.AccessKey, creds.SecretKey)
	if out, err := alias.CombinedOutput(); err != nil {
		t.Fatalf("mc alias: %v %s", err, out)
	}
	cmd := exec.Command(mcPath, "--config-dir", dir, "cp", src, dst)
	cmd.Env = append(os.Environ(), extra...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mc cp: %v %s", err, out)
	}
}

func assertStored(t *testing.T, base string, creds auth.Credentials, path string, want []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || !bytes.Equal(body, want) {
		t.Fatalf("GET %s %d len %d", path, resp.StatusCode, len(body))
	}
}
