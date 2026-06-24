package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestWORMRetentionBlocksDelete(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()

	token, err := srv.AdminToken()
	if err != nil {
		t.Fatal(err)
	}

	bucket := "worm-bucket"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	brec, _ := srv.Meta().GetBucket(bucket)
	brec.ObjectLock = true
	brec.RetentionDays = 30
	if err := srv.Meta().UpdateBucket(brec); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewReader([]byte("immutable-data"))
	_, err = srv.Svc().PutObject(ctx, bucket, "locked.txt", body, int64(body.Len()), "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/buckets/"+bucket+"/objects/locked.txt", nil)
	delReq.SetPathValue("bucket", bucket)
	delReq.SetPathValue("key", "locked.txt")
	delReq.Header.Set("Authorization", "Bearer "+token)
	delW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delW, delReq)
	if delW.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for retention lock, got %d: %s", delW.Code, delW.Body.String())
	}
}

func TestLegalHoldBlocksDelete(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()

	bucket := "hold-bucket"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader([]byte("held"))
	_, err := srv.Svc().PutObject(ctx, bucket, "doc.txt", body, int64(body.Len()), "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Meta().SetObjectLegalHold(bucket, "doc.txt", "", true); err != nil {
		t.Fatal(err)
	}

	err = srv.Svc().DeleteObject(ctx, bucket, "doc.txt", "")
	if err != metadata.ErrLegalHold {
		t.Fatalf("expected legal hold error, got %v", err)
	}
}

func TestTenantCRUD(t *testing.T) {
	srv := testServer(t)
	token := loginToken(t, srv, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/tenants", token, []byte(`{"name":"Team A"}`))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create tenant: %d %s", w.Code, w.Body.String())
	}

	listReq := authReq(http.MethodGet, "/api/v1/tenants", token, nil)
	listW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("list tenants: %d", listW.Code)
	}
	var resp struct {
		Tenants []metadata.TenantRecord `json:"tenants"`
	}
	if err := json.Unmarshal(listW.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Tenants) < 2 {
		t.Fatalf("expected default + new tenant, got %d", len(resp.Tenants))
	}
}

func TestClusterStatus(t *testing.T) {
	srv := testServer(t)
	token := loginToken(t, srv, "admin", "admin")
	req := authReq(http.MethodGet, "/api/v1/cluster/status", token, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cluster status: %d", w.Code)
	}
}

func TestSystemConfigLDAPFields(t *testing.T) {
	srv := testServer(t)
	token := loginToken(t, srv, "admin", "admin")

	cfg := metadata.SystemConfig{
		SoftDeleteEnabled:  true,
		TrashRetentionDays: 30,
		LDAP: metadata.LDAPConfig{
			Enabled: true,
			URL:     "ldap://localhost",
			BaseDN:  "dc=example,dc=com",
		},
	}
	body, _ := json.Marshal(cfg)
	putReq := authReq(http.MethodPut, "/api/v1/settings/system", token, body)
	putW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("put config: %d %s", putW.Code, putW.Body.String())
	}

	getReq := authReq(http.MethodGet, "/api/v1/settings/system", token, nil)
	getW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getW, getReq)
	var got metadata.SystemConfig
	if err := json.Unmarshal(getW.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.LDAP.Enabled || got.LDAP.URL != "ldap://localhost" {
		t.Fatalf("ldap config not persisted: %+v", got.LDAP)
	}
}

func TestStorageClassOnBucket(t *testing.T) {
	srv := testServer(t)
	ctx := context.Background()
	token := loginToken(t, srv, "admin", "admin")

	bucket := "cold-bucket"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	updateBody := []byte(`{"storage_class":"cold","object_lock_enabled":false}`)
	req := authReq(http.MethodPut, "/api/v1/settings/buckets/"+bucket, token, updateBody)
	req.SetPathValue("name", bucket)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update bucket settings: %d %s", w.Code, w.Body.String())
	}

	brec, err := srv.Meta().GetBucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if brec.StorageClass != metadata.StorageClassCold {
		t.Fatalf("expected cold storage class, got %s", brec.StorageClass)
	}
}
