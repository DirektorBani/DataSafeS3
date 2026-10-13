package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/cluster/pki"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestClusterPeerEndpointsRequireProof(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{
		"/api/v1/clusters/local/rotate",
		"/api/v1/clusters/local/revoke-notify",
	} {
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(`{}`)))
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status %d body %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestClusterPeerSignatureAllowsRotate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STORAGE_CLUSTER_CERT_DIR", dir)
	s := testServer(t)
	mgr := pki.NewManager(dir)
	if err := mgr.EnsureCA(); err != nil {
		t.Fatal(err)
	}
	csr, key, err := mgr.GenerateCSR("peer-a")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, serial, nb, na, err := mgr.SignCSR(csr, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.SaveClientCert("peer-a", certPEM, key); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := s.Meta().PutTrustedCluster(metadata.TrustedCluster{
		ID: "peer-a", Name: "Peer", Endpoint: "http://127.0.0.1:9",
		AuthMode: metadata.TrustedClusterAuthMTLS, Status: metadata.TrustedClusterStatusHealthy,
		CreatedAt: now, Active: true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Meta().PutClusterCertificate(metadata.ClusterCertificate{
		Serial: serial, ClusterID: "peer-a", Role: metadata.ClusterCertRoleClient,
		NotBefore: nb, NotAfter: na,
	}); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/local/rotate", bytes.NewReader(body))
	sig, err := pki.SignCanonical(key, pki.CanonicalClusterRequest(http.MethodPost, "/api/v1/clusters/local/rotate", body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-DataSafe-Cluster-Cert", base64.StdEncoding.EncodeToString(certPEM))
	req.Header.Set("X-DataSafe-Cluster-Sig", base64.StdEncoding.EncodeToString(sig))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code == http.StatusUnauthorized {
		t.Fatalf("signed peer was rejected: %s", rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty csr, got %d %s", rec.Code, rec.Body.String())
	}
}

func TestPresignAccessControls(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "presign-user", auth.RoleUser)

	req := authReq(http.MethodPost, "/api/v1/buckets/owned", adminTok, []byte(`{"visibility":"private"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	body, _ := json.Marshal(map[string]any{"method": "GET", "bucket": "owned", "key": "a", "expires_seconds": 60})
	req = authReq(http.MethodPost, "/api/v1/presign", userTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("presign without access %d %s", rec.Code, rec.Body.String())
	}

	body, _ = json.Marshal(map[string]any{"method": "PUT", "bucket": "owned", "key": "a", "expires_seconds": 60})
	req = authReq(http.MethodPost, "/api/v1/presign", userTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("presign put without write %d %s", rec.Code, rec.Body.String())
	}

	body, _ = json.Marshal(map[string]any{"method": "GET", "bucket": "owned", "key": "a", "expires_seconds": 8 * 24 * 3600})
	req = authReq(http.MethodPost, "/api/v1/presign", adminTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presign 8 days %d %s", rec.Code, rec.Body.String())
	}

	body, _ = json.Marshal(map[string]any{"method": "DELETE", "bucket": "owned", "key": "a", "expires_seconds": 60})
	req = authReq(http.MethodPost, "/api/v1/presign", adminTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("presign delete %d %s", rec.Code, rec.Body.String())
	}
}

func TestOrdinaryUserCannotCreatePublicRead(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	userTok := createUser(t, s, adminTok, "pub-user", auth.RoleUser)
	req := authReq(http.MethodPost, "/api/v1/buckets/open-photos", userTok, []byte(`{"visibility":"public-read"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("public-read create %d %s", rec.Code, rec.Body.String())
	}
}

func TestUnsupportedPolicyConditionRejected(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/pol", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	doc := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":"*","Action":"s3:GetObject","Resource":"arn:aws:s3:::pol/*","Condition":{"StringEquals":{"s3:prefix":"a"}}}]}`
	body, _ := json.Marshal(map[string]string{"policy": doc})
	req = authReq(http.MethodPut, "/api/v1/buckets/pol/policy", adminTok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("policy %d %s", rec.Code, rec.Body.String())
	}
}

func TestShareDisableBlocksThenRestores(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/sharebox", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	put := authReq(http.MethodPut, "/api/v1/buckets/sharebox/objects/note.txt", tok, []byte("hello"))
	put.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, put)

	body, _ := json.Marshal(map[string]any{"key": "note.txt", "expires_in_sec": 3600, "password": "secret"})
	req = authReq(http.MethodPost, "/api/v1/buckets/sharebox/shares", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create share %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			Token       string `json:"token"`
			HasPassword bool   `json:"has_password"`
		} `json:"share"`
		URL string `json:"url"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if !created.Share.HasPassword || created.Share.Token == "" {
		t.Fatalf("share response %+v", created)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("password_hash")) {
		t.Fatal("password hash leaked")
	}

	off, _ := json.Marshal(map[string]any{"share_links_allowed": false})
	req = authReq(http.MethodPut, "/api/v1/buckets/sharebox/sharing", tok, off)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable sharing %d %s", rec.Code, rec.Body.String())
	}

	pub := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, pub)
	if rec.Code != http.StatusForbidden || !bytes.Contains(rec.Body.Bytes(), []byte("отключ")) {
		t.Fatalf("blocked share %d %s", rec.Code, rec.Body.String())
	}

	on, _ := json.Marshal(map[string]any{"share_links_allowed": true})
	req = authReq(http.MethodPut, "/api/v1/buckets/sharebox/sharing", tok, on)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable sharing %d %s", rec.Code, rec.Body.String())
	}
	pub = httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
	pub.Header.Set("X-DataSafe-Share-Password", "secret")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, pub)
	if rec.Code != http.StatusOK {
		t.Fatalf("restored share %d %s", rec.Code, rec.Body.String())
	}
	got, _ := io.ReadAll(rec.Body)
	if string(got) != "hello" {
		t.Fatalf("body %q", got)
	}
}

func TestBlockPublicAccessHidesExistingBucket(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := authReq(http.MethodPost, "/api/v1/buckets/photos", tok, []byte(`{"visibility":"public-read"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	put := authReq(http.MethodPut, "/api/v1/buckets/photos/objects/note.txt", tok, []byte("FROM-A"))
	put.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, put)

	body, _ := json.Marshal(map[string]any{"public_access_blocked": true})
	req = authReq(http.MethodPut, "/api/v1/settings/sharing", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bpa %d %s", rec.Code, rec.Body.String())
	}
	anon := httptest.NewRequest(http.MethodGet, "/photos/note.txt", nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, anon)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous after BPA %d %s", rec.Code, rec.Body.String())
	}
}

func TestAssumeRoleRejectsRoleArn(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]any{"role_arn": "arn:aws:iam::123:role/nope"})
	req := authReq(http.MethodPost, "/api/v1/sts/assume-role", tok, body)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("assume role %d %s", rec.Code, rec.Body.String())
	}
	body, _ = json.Marshal(map[string]any{"role_session_name": "s"})
	req = authReq(http.MethodPost, "/api/v1/sts/assume-role", tok, body)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("assume empty arn %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("arn:aws:iam::datasafe:user/admin")) {
		t.Fatalf("arn not rewritten: %s", rec.Body.String())
	}
}

func TestOperatorWriteRequiresGrant(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")
	opTok := createUser(t, s, adminTok, "op-nowrite", auth.RoleOperator)
	req := authReq(http.MethodPost, "/api/v1/buckets/opbox", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	put := authReq(http.MethodPut, "/api/v1/buckets/opbox/objects/a.txt", opTok, []byte("no"))
	put.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, put)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator write %d %s", rec.Code, rec.Body.String())
	}
}
