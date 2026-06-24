package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestS3BucketVersioningXML(t *testing.T) {
	s := testServer(t)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	creds := auth.Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}

	mb, _ := http.NewRequest(http.MethodPut, ts.URL+"/verxml", nil)
	_ = auth.SignRequest(mb, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD")
	resp, err := http.DefaultClient.Do(mb)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create bucket %d", resp.StatusCode)
	}

	body := `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`
	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/verxml?versioning", strings.NewReader(body))
	putReq.Header.Set("Content-Type", "application/xml")
	if err := auth.SignRequest(putReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	putResp, err := http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("put versioning %d", putResp.StatusCode)
	}

	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/verxml?versioning", nil)
	if err := auth.SignRequest(getReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get versioning %d", getResp.StatusCode)
	}
	if !strings.Contains(buf.String(), "<Status>Enabled</Status>") {
		t.Fatalf("expected Enabled status: %s", buf.String())
	}
}

func TestQuotaEnforcement(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	// Set admin user quota to 10 bytes
	updateBody := []byte(`{"max_size_bytes":10}`)
	req := authReq(http.MethodPut, "/api/v1/users/admin-bootstrap", adminTok, updateBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set quota %d %s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/qbucket", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	uploadKey := func(key, content string) int {
		t.Helper()
		req := authReq(http.MethodPut, "/api/v1/buckets/qbucket/objects/"+key, adminTok, []byte(content))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := uploadKey("big.bin", "123456789012345"); code != http.StatusForbidden {
		t.Fatalf("expected quota exceeded 403 got %d", code)
	}
	if code := uploadKey("a.txt", "abc"); code != http.StatusOK {
		t.Fatalf("small upload should succeed got %d", code)
	}
	if code := uploadKey("b.txt", "defghijk"); code != http.StatusForbidden {
		t.Fatalf("expected quota exceeded on cumulative upload got %d", code)
	}
}

func TestQuotaOverwriteNetDelta(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	updateBody := []byte(`{"max_size_bytes":10}`)
	req := authReq(http.MethodPut, "/api/v1/users/admin-bootstrap", adminTok, updateBody)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set quota %d", rec.Code)
	}

	req = authReq(http.MethodPost, "/api/v1/buckets/qow", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d", rec.Code)
	}

	upload := func(key, content string) int {
		t.Helper()
		req := authReq(http.MethodPut, "/api/v1/buckets/qow/objects/"+key, adminTok, []byte(content))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := upload("a.txt", "12345"); code != http.StatusOK {
		t.Fatalf("initial upload %d", code)
	}
	if code := upload("a.txt", "1234567890"); code != http.StatusOK {
		t.Fatalf("overwrite within quota should succeed, got %d", code)
	}
	if code := upload("a.txt", "12345678901"); code != http.StatusForbidden {
		t.Fatalf("overwrite exceeding quota should fail, got %d", code)
	}
}
