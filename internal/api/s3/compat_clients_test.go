package s3_test

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

// TestS3ClientCompatibility drives the real server with the clients operators
// use. A missing binary is skipped unless S3_COMPAT_REQUIRE lists it
// (comma-separated: aws,mc,rclone,restic).
func TestS3ClientCompatibility(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	setupBucket(t, ts, creds, "compat")

	ran := 0
	if tool(t, "aws") != "" {
		ran++
		runAWSCompat(t, ts.URL, creds)
	}
	if tool(t, "mc") != "" {
		ran++
		runMCCompat(t, ts.URL, creds)
	}
	if tool(t, "rclone") != "" {
		ran++
		runRcloneCompat(t, ts.URL, creds)
	}
	if tool(t, "restic") != "" {
		ran++
		runResticCompat(t, ts.URL, creds)
	}
	if ran == 0 {
		t.Log("no S3 clients on PATH; DeleteObjects unit tests cover the server")
	}
}

func tool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	required := false
	for _, part := range strings.Split(os.Getenv("S3_COMPAT_REQUIRE"), ",") {
		if strings.TrimSpace(part) == name {
			required = true
		}
	}
	if err != nil {
		if required {
			t.Fatalf("%s is required by S3_COMPAT_REQUIRE but is not on PATH", name)
		}
		t.Logf("skip %s: not on PATH", name)
		return ""
	}
	return path
}

func runAWSCompat(t *testing.T, endpoint string, creds auth.Credentials) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "aws.config")
	if err := os.WriteFile(cfg, []byte("[default]\nregion = us-east-1\ns3 =\n    addressing_style = path\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	aws := func(args ...string) string {
		t.Helper()
		return runTool(t, endpoint, creds, cfg, "aws", args...)
	}
	payload := []byte("compat-aws")
	path := filepath.Join(dir, "obj.bin")
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	aws("s3api", "head-bucket", "--bucket", "compat")
	aws("s3", "cp", path, "s3://compat/hello.txt")
	gotPath := filepath.Join(dir, "got.bin")
	aws("s3", "cp", "s3://compat/hello.txt", gotPath)
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("aws get %q", got)
	}
	aws("s3api", "head-object", "--bucket", "compat", "--key", "hello.txt")
	listed := aws("s3api", "list-objects-v2", "--bucket", "compat")
	if !strings.Contains(listed, "hello.txt") {
		t.Fatalf("list v2: %s", listed)
	}
	aws("s3", "cp", "s3://compat/hello.txt", "s3://compat/copied.txt")
	url := strings.TrimSpace(aws("s3", "presign", "s3://compat/hello.txt", "--expires-in", "300"))
	if !strings.Contains(url, "X-Amz-Signature") {
		t.Fatalf("presign %q", url)
	}
	body := runPlain(t, "curl", "-fsS", url)
	if strings.TrimSpace(body) != string(payload) {
		t.Fatalf("presigned get %q", body)
	}

	part := filepath.Join(dir, "part.bin")
	if err := os.WriteFile(part, []byte("mp-part"), 0o644); err != nil {
		t.Fatal(err)
	}
	initOut := aws("s3api", "create-multipart-upload", "--bucket", "compat", "--key", "multi.bin")
	uploadID := compatJSONField(initOut, "UploadId")
	if uploadID == "" {
		t.Fatalf("multipart init: %s", initOut)
	}
	up := aws("s3api", "upload-part", "--bucket", "compat", "--key", "multi.bin", "--part-number", "1", "--upload-id", uploadID, "--body", part)
	etag := compatJSONField(up, "ETag")
	if etag == "" {
		t.Fatalf("upload part: %s", up)
	}
	complete, err := json.Marshal(map[string]any{
		"Parts": []map[string]any{{"ETag": etag, "PartNumber": 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	compPath := filepath.Join(dir, "complete.json")
	if err := os.WriteFile(compPath, complete, 0o644); err != nil {
		t.Fatal(err)
	}
	aws("s3api", "complete-multipart-upload", "--bucket", "compat", "--key", "multi.bin", "--upload-id", uploadID, "--multipart-upload", "file://"+compPath)

	aws("s3api", "put-bucket-versioning", "--bucket", "compat", "--versioning-configuration", "Status=Enabled")
	aws("s3", "cp", path, "s3://compat/ver.txt")
	aws("s3", "rm", "s3://compat/ver.txt")
	versions := aws("s3api", "list-object-versions", "--bucket", "compat", "--prefix", "ver.txt")
	if !strings.Contains(versions, "DeleteMarker") && !strings.Contains(versions, "DeleteMarkers") {
		t.Fatalf("versioned delete: %s", versions)
	}

	sub := filepath.Join(dir, "tree")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	aws("s3", "cp", sub, "s3://compat/tree/", "--recursive")
	aws("s3", "rm", "s3://compat/tree/", "--recursive")
	left := aws("s3api", "list-objects-v2", "--bucket", "compat", "--prefix", "tree/")
	if strings.Contains(left, "a.txt") || strings.Contains(left, "b.txt") {
		t.Fatalf("recursive delete left objects: %s", left)
	}
}

func runMCCompat(t *testing.T, endpoint string, creds auth.Credentials) {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "mc")
	runTool(t, endpoint, creds, "", "mc", "--config-dir", cfg, "alias", "set", "ds", endpoint, creds.AccessKey, creds.SecretKey, "--api", "s3v4", "--path", "on")
	runTool(t, endpoint, creds, "", "mc", "--config-dir", cfg, "mb", "ds/mcbucket")
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("mc-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTool(t, endpoint, creds, "", "mc", "--config-dir", cfg, "cp", file, "ds/mcbucket/f.txt")
	listed := runTool(t, endpoint, creds, "", "mc", "--config-dir", cfg, "ls", "ds/mcbucket")
	if !strings.Contains(listed, "f.txt") {
		t.Fatalf("mc ls: %s", listed)
	}
	runTool(t, endpoint, creds, "", "mc", "--config-dir", cfg, "rm", "--recursive", "--force", "ds/mcbucket")
}

func runRcloneCompat(t *testing.T, endpoint string, creds auth.Credentials) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "r.txt")
	if err := os.WriteFile(file, []byte("rclone-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"RCLONE_CONFIG_DS_TYPE=s3",
		"RCLONE_CONFIG_DS_PROVIDER=Other",
		"RCLONE_CONFIG_DS_ACCESS_KEY_ID=" + creds.AccessKey,
		"RCLONE_CONFIG_DS_SECRET_ACCESS_KEY=" + creds.SecretKey,
		"RCLONE_CONFIG_DS_ENDPOINT=" + endpoint,
		"RCLONE_CONFIG_DS_FORCE_PATH_STYLE=true",
		"RCLONE_CONFIG_DS_REGION=us-east-1",
	}
	runToolEnv(t, env, "rclone", "copyto", file, "ds:compat/rclone.txt")
	sub := filepath.Join(dir, "more")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	runToolEnv(t, env, "rclone", "copy", sub, "ds:compat/rctree")
	listed := runToolEnv(t, env, "rclone", "ls", "ds:compat")
	if !strings.Contains(listed, "rclone.txt") || !strings.Contains(listed, "a.txt") {
		t.Fatalf("rclone ls: %s", listed)
	}
	runToolEnv(t, env, "rclone", "deletefile", "ds:compat/rclone.txt")
	runToolEnv(t, env, "rclone", "delete", "ds:compat/rctree")
	left := runToolEnv(t, env, "rclone", "ls", "ds:compat/rctree")
	if strings.Contains(left, "a.txt") || strings.Contains(left, "b.txt") {
		t.Fatalf("rclone delete left objects: %s", left)
	}
}

func runResticCompat(t *testing.T, endpoint string, creds auth.Credentials) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(file, []byte("restic-body"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"RESTIC_REPOSITORY=s3:" + endpoint + "/compat/restic",
		"RESTIC_PASSWORD=compat-restic-password",
		"AWS_ACCESS_KEY_ID=" + creds.AccessKey,
		"AWS_SECRET_ACCESS_KEY=" + creds.SecretKey,
		"AWS_DEFAULT_REGION=us-east-1",
	}
	runToolEnv(t, env, "restic", "init")
	runToolEnv(t, env, "restic", "backup", file)
	snaps := runToolEnv(t, env, "restic", "snapshots")
	if !strings.Contains(snaps, "note.txt") && !strings.Contains(snaps, "snapshot") {
		t.Fatalf("restic snapshots: %s", snaps)
	}
	runToolEnv(t, env, "restic", "backup", file)
	runToolEnv(t, env, "restic", "forget", "--keep-last", "1", "--prune")
}

func runTool(t *testing.T, endpoint string, creds auth.Credentials, awsConfig, name string, args ...string) string {
	t.Helper()
	extra := []string{
		"AWS_ACCESS_KEY_ID=" + creds.AccessKey,
		"AWS_SECRET_ACCESS_KEY=" + creds.SecretKey,
		"AWS_DEFAULT_REGION=us-east-1",
		"AWS_EC2_METADATA_DISABLED=true",
	}
	if awsConfig != "" {
		extra = append(extra, "AWS_CONFIG_FILE="+awsConfig)
	}
	if name == "aws" {
		args = append([]string{"--endpoint-url", endpoint}, args...)
	}
	return runToolEnv(t, extra, name, args...)
}

func runToolEnv(t *testing.T, extra []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "AWS_REQUEST_CHECKSUM_CALCULATION=") || strings.HasPrefix(e, "AWS_RESPONSE_CHECKSUM_VALIDATION=") {
			continue
		}
		env = append(env, e)
	}
	cmd.Env = append(env, extra...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func runPlain(t *testing.T, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func compatJSONField(s, key string) string {
	var m map[string]any
	raw := s
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			raw = s[i : j+1]
		}
	}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
