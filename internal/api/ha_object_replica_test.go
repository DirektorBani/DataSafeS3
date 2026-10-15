package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
