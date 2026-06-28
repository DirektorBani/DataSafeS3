package s3_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
)

func testServer(t *testing.T) (*api.Server, auth.Credentials) {
	t.Helper()
	dir := t.TempDir()
	srv, err := api.NewServer(api.Config{
		DataDir:   dir,
		Region:    "us-east-1",
		AccessKey: "testkey",
		SecretKey: "testsecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	cfg, err := srv.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := srv.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return srv, auth.Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
}

func signRequest(t *testing.T, req *http.Request, creds auth.Credentials, region string) {
	t.Helper()
	if req.Host == "" {
		req.Host = req.URL.Host
	}
	if err := auth.SignRequest(req, creds, region, "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
}

func TestS3BucketObjectFlow(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Create bucket
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/mybucket", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create bucket status %d", resp.StatusCode)
	}

	// Put object
	body := bytes.NewReader([]byte("payload"))
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/mybucket/hello.txt", body)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put object status %d", resp.StatusCode)
	}

	// Get object
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/mybucket/hello.txt", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get object status %d", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "payload" {
		t.Fatalf("got %q", got)
	}

	// Head object
	req, _ = http.NewRequest(http.MethodHead, ts.URL+"/mybucket/hello.txt", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("head status %d", resp.StatusCode)
	}

	// List buckets
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list buckets status %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(data), "mybucket") {
		t.Fatalf("expected bucket in list: %s", data)
	}
}

func TestHealthEndpoints(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	for _, path := range []string{"/healthz", "/api/v1/health"} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status %d", path, resp.StatusCode)
		}
	}
}

func TestCopyObject(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, u := range []string{"/b", "/b/src.txt"} {
		method := http.MethodPut
		var body io.Reader
		if strings.Contains(u, ".txt") {
			body = bytes.NewReader([]byte("copy-me"))
		}
		req, _ := http.NewRequest(method, ts.URL+u, body)
		signRequest(t, req, creds, "us-east-1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/b/dst.txt", nil)
	req.Header.Set("x-amz-copy-source", "/b/src.txt")
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("copy status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/b/dst.txt", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "copy-me" {
		t.Fatalf("got %q", got)
	}
}

func TestS3PutObject_rejectsTraversalKey(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/mybucket/../../etc/passwd", strings.NewReader("x"))
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func init() {
	_ = context.Background()
	_ = filepath.Join
}
