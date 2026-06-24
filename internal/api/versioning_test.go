package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestBucketVersioningViaAPI(t *testing.T) {
	s := testServer(t)
	adminTok := loginToken(t, s, "admin", "admin")

	req := authReq(http.MethodPost, "/api/v1/buckets/versioned", adminTok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create bucket %d %s", rec.Code, rec.Body.String())
	}

	settingsBody, _ := json.Marshal(map[string]any{"versioning_enabled": true})
	req = authReq(http.MethodPut, "/api/v1/settings/buckets/versioned", adminTok, settingsBody)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("enable versioning %d %s", rec.Code, rec.Body.String())
	}

	upload := func(key, content string) {
		t.Helper()
		req := authReq(http.MethodPut, "/api/v1/buckets/versioned/objects/"+key, adminTok, []byte(content))
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("upload %s status %d %s", key, rec.Code, rec.Body.String())
		}
	}
	upload("doc.txt", "version-one")
	upload("doc.txt", "version-two-longer")

	req = authReq(http.MethodGet, "/api/v1/buckets/versioned/objects", adminTok, nil)
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list objects %d", rec.Code)
	}
	var listResp struct {
		Objects []struct {
			Key       string `json:"key"`
			Size      int64  `json:"size"`
			VersionID string `json:"version_id"`
		} `json:"objects"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &listResp)
	if len(listResp.Objects) != 1 {
		t.Fatalf("expected 1 latest object got %d", len(listResp.Objects))
	}
	if listResp.Objects[0].Size != int64(len("version-two-longer")) {
		t.Fatalf("latest size %d", listResp.Objects[0].Size)
	}

	// S3 ListObjectVersions
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	creds := auth.Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/versioned?versions", nil)
	if err := auth.SignRequest(listReq, creds, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	listRec, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	defer listRec.Body.Close()
	body := new(bytes.Buffer)
	_, _ = body.ReadFrom(listRec.Body)
	if listRec.StatusCode != http.StatusOK {
		t.Fatalf("list versions status %d", listRec.StatusCode)
	}
	if strings.Count(body.String(), "<VersionId>") < 2 {
		t.Fatalf("expected 2 versions in XML: %s", body.String())
	}
}
