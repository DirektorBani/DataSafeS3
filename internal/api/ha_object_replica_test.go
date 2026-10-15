package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestHandleObjectReplicaWritesStoredBytes(t *testing.T) {
	t.Setenv("STORAGE_HA_ENABLED", "false")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	s, err := NewServer(Config{
		DataDir: t.TempDir(), AdminUser: "admin", AdminPassword: "admin", JWTSecret: "replica-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	body := "stored-bytes"
	req := httptest.NewRequest(http.MethodPut, "/api/v1/ha/object-replica?bucket=box&key=a.txt", strings.NewReader(body))
	req.Header.Set("X-DataSafe-Replica", replicaMAC("replica-secret", "box", "a.txt", ""))
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("replica %d %s", rr.Code, rr.Body.String())
	}
	rc, _, err := s.backend.GetObject(context.Background(), "box", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Fatalf("stored %q", got)
	}

	bad := httptest.NewRequest(http.MethodPut, "/api/v1/ha/object-replica?bucket=box&key=a.txt", strings.NewReader("nope"))
	bad.Header.Set("X-DataSafe-Replica", replicaMAC("other", "box", "a.txt", ""))
	rr = httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, bad)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bad mac %d", rr.Code)
	}
}

func TestGetObjectPullsMissingBytesFromPeer(t *testing.T) {
	t.Setenv("STORAGE_HA_ENABLED", "false")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	holder := newReplicaLab(t)
	empty := newReplicaLab(t)
	reader := newReplicaLab(t)
	body := "stored-bytes"
	putReplica(t, holder, "box", "a.txt", "", body)
	putReplica(t, holder, "box", "b.txt", "v1", "versioned-bytes")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ha/object-replica?bucket=box&key=a.txt", nil)
	req.Header.Set("X-DataSafe-Replica", replicaMAC("replica-secret", "box", "a.txt", ""))
	rr := httptest.NewRecorder()
	holder.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != body {
		t.Fatalf("get %d %q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Fatalf("content-length %s", rr.Header().Get("Content-Length"))
	}
	bad := httptest.NewRequest(http.MethodGet, "/api/v1/ha/object-replica?bucket=box&key=a.txt", nil)
	bad.Header.Set("X-DataSafe-Replica", replicaMAC("other", "box", "a.txt", ""))
	rr = httptest.NewRecorder()
	holder.Handler().ServeHTTP(rr, bad)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bad mac %d", rr.Code)
	}
	miss := httptest.NewRequest(http.MethodGet, "/api/v1/ha/object-replica?bucket=box&key=missing.txt", nil)
	miss.Header.Set("X-DataSafe-Replica", replicaMAC("replica-secret", "box", "missing.txt", ""))
	rr = httptest.NewRecorder()
	holder.Handler().ServeHTTP(rr, miss)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing %d", rr.Code)
	}

	holderSrv := httptest.NewServer(holder.Handler())
	t.Cleanup(holderSrv.Close)
	emptySrv := httptest.NewServer(empty.Handler())
	t.Cleanup(emptySrv.Close)

	if err := reader.meta.PutBucket(metadata.BucketRecord{Name: "box", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := reader.meta.PutObject(metadata.ObjectRecord{
		Bucket: "box", Key: "a.txt", Size: int64(len(body)),
		Encryption: &metadata.ObjectEncryption{Plaintext: true},
	}); err != nil {
		t.Fatal(err)
	}
	reader.svc.FetchStoredObject = func(ctx context.Context, bucket, key, versionID string) error {
		return reader.pullFromPeers(ctx, []string{emptySrv.URL, holderSrv.URL}, bucket, key, versionID)
	}
	rc, _, err := reader.svc.GetObject(context.Background(), "box", "a.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Fatalf("pulled %q", got)
	}
	if err := reader.pullObjectReplica(context.Background(), holderSrv.URL, "box", "b.txt", "v1"); err != nil {
		t.Fatal(err)
	}
	rc, _, err = reader.backend.GetObjectVersion(context.Background(), "box", "b.txt", "v1")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ = io.ReadAll(rc)
	if string(got) != "versioned-bytes" {
		t.Fatalf("version %q", got)
	}
}

func newReplicaLab(t *testing.T) *Server {
	t.Helper()
	s, err := NewServer(Config{
		DataDir: t.TempDir(), AdminUser: "admin", AdminPassword: "admin", JWTSecret: "replica-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func putReplica(t *testing.T, s *Server, bucket, key, version, body string) {
	t.Helper()
	u := "/api/v1/ha/object-replica?bucket=" + url.QueryEscape(bucket) + "&key=" + url.QueryEscape(key)
	if version != "" {
		u += "&version=" + url.QueryEscape(version)
	}
	req := httptest.NewRequest(http.MethodPut, u, strings.NewReader(body))
	req.Header.Set("X-DataSafe-Replica", replicaMAC(s.cfg.JWTSecret, bucket, key, version))
	rr := httptest.NewRecorder()
	s.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("put %s %d %s", key, rr.Code, rr.Body.String())
	}
}

func TestIsHAObjectRead(t *testing.T) {
	if isHAObjectRead(httptest.NewRequest(http.MethodGet, "/box", nil)) {
		t.Fatal("bucket list is not an object read")
	}
	if !isHAObjectRead(httptest.NewRequest(http.MethodGet, "/box/a.txt", nil)) {
		t.Fatal("object get")
	}
	if !isHAObjectRead(httptest.NewRequest(http.MethodHead, "/box/a.txt", nil)) {
		t.Fatal("object head")
	}
	if isHAObjectRead(httptest.NewRequest(http.MethodPut, "/box/a.txt", nil)) {
		t.Fatal("put is a write")
	}
}
