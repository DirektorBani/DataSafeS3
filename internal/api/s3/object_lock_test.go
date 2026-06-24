package s3_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestObjectLockConfigurationXML(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bucket := "lock-bucket"
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket, nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	lockXML := `<?xml version="1.0" encoding="UTF-8"?>
<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <ObjectLockEnabled>Enabled</ObjectLockEnabled>
  <Rule>
    <DefaultRetention>
      <Mode>GOVERNANCE</Mode>
      <Days>30</Days>
    </DefaultRetention>
  </Rule>
</ObjectLockConfiguration>`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"?object-lock", strings.NewReader(lockXML))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put object lock config status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/"+bucket+"?object-lock", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get object lock status %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "Enabled") || !strings.Contains(string(body), "GOVERNANCE") {
		t.Fatalf("unexpected object lock xml: %s", body)
	}
}

func TestObjectRetentionBlocksDeleteS3(t *testing.T) {
	srv, creds := testServer(t)
	ctx := context.Background()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bucket := "retention-bucket"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	brec, _ := srv.Meta().GetBucket(bucket)
	brec.ObjectLock = true
	if err := srv.Meta().UpdateBucket(brec); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewReader([]byte("locked-data"))
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"/file.txt", body)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	until := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	retXML := `<Retention><Mode>GOVERNANCE</Mode><RetainUntilDate>` + until + `</RetainUntilDate></Retention>`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"/file.txt?retention", strings.NewReader(retXML))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put retention status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodDelete, ts.URL+"/"+bucket+"/file.txt", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for retention lock, got %d: %s", resp.StatusCode, respBody)
	}
	if !strings.Contains(string(respBody), "AccessDenied") {
		t.Fatalf("expected AccessDenied xml: %s", respBody)
	}
}

func TestLegalHoldS3XML(t *testing.T) {
	srv, creds := testServer(t)
	ctx := context.Background()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bucket := "legal-hold-bucket"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader([]byte("held"))
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"/doc.txt", body)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	holdXML := `<LegalHold><Status>ON</Status></LegalHold>`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"/doc.txt?legal-hold", strings.NewReader(holdXML))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put legal hold status %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/"+bucket+"/doc.txt?legal-hold", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(respBody), "ON") {
		t.Fatalf("expected ON legal hold: %s", respBody)
	}

	err = srv.Svc().DeleteObject(ctx, bucket, "doc.txt", "")
	if err != metadata.ErrLegalHold {
		t.Fatalf("expected legal hold error, got %v", err)
	}
}

func TestListObjectsStorageClass(t *testing.T) {
	srv, creds := testServer(t)
	ctx := context.Background()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bucket := "sc-bucket"
	if err := srv.Svc().CreateBucket(ctx, bucket, "admin"); err != nil {
		t.Fatal(err)
	}
	brec, _ := srv.Meta().GetBucket(bucket)
	brec.StorageClass = metadata.StorageClassWarm
	if err := srv.Meta().UpdateBucket(brec); err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader([]byte("data"))
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"/obj.txt", body)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/"+bucket+"?list-type=2", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(respBody), "STANDARD_IA") {
		t.Fatalf("expected STANDARD_IA in list: %s", respBody)
	}
}
