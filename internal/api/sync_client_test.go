package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/syncapp"
)

func TestDatasafeSyncPushPull(t *testing.T) {
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]string{
		"username": "syncuser", "password": "usr123", "role": auth.RoleUser, "email": "sync@test.com",
	})
	req := authReq(http.MethodPost, "/api/v1/users", adminTok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create user %d", rec.Code)
	}
	userTok := loginToken(t, s, "syncuser", "usr123")

	req = authReq(http.MethodPost, "/api/v1/buckets/syncbox", userTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	c := syncapp.NewClient(ts.URL, userTok)
	dir := t.TempDir()
	local := filepath.Join(dir, "sync")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "note.txt"), []byte("synced"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := syncapp.RunOnce(syncapp.Options{
		ProfileName: "itest",
		Client:      c,
		Folder:      local,
		Bucket:      "syncbox",
		Push:        true,
		Pull:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 1 {
		t.Fatalf("expected 1 upload, got %+v", res)
	}

	_ = os.Remove(filepath.Join(local, "note.txt"))
	res, err = syncapp.RunOnce(syncapp.Options{
		ProfileName: "itest",
		Client:      c,
		Folder:      local,
		Bucket:      "syncbox",
		Push:        false,
		Pull:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Downloaded != 1 {
		t.Fatalf("expected 1 download, got %+v", res)
	}
	data, err := os.ReadFile(filepath.Join(local, "note.txt"))
	if err != nil || string(data) != "synced" {
		t.Fatalf("file content %q err=%v", data, err)
	}
}

func TestDatasafeSyncDeletePropagation(t *testing.T) {
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	s := testServer(t)
	userTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/delbox", userTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	c := syncapp.NewClient(ts.URL, userTok)
	local := filepath.Join(t.TempDir(), "sync")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "gone.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := syncapp.RunOnce(syncapp.Options{
		ProfileName: "deltest",
		Client:      c,
		Folder:      local,
		Bucket:      "delbox",
		Push:        true,
		Pull:        true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(local, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	res, err := syncapp.RunOnce(syncapp.Options{
		ProfileName: "deltest",
		Client:      c,
		Folder:      local,
		Bucket:      "delbox",
		Push:        true,
		Pull:        false,
		DeleteSync:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.DeletedRemote != 1 {
		t.Fatalf("expected remote delete, got %+v", res)
	}

	objs, err := c.ListAllObjects("delbox", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 0 {
		t.Fatalf("expected empty bucket, got %d objects", len(objs))
	}
}
