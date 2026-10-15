package s3_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestListMultipartUploadsSeesActiveUpload(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// A cluster bucket's storage key is not the logical name. The upload row
	// is stored under the storage key. Listing by the name the client sends
	// must still return it.
	const logical = "photos"
	const storageKey = "o__user1__photos"
	if err := srv.Meta().PutBucket(metadata.BucketRecord{
		Name: logical, StorageKey: storageKey, Owner: "alice", OwnerID: "user1",
		CreatedAt: time.Now().UTC(), TenantID: metadata.DefaultTenantID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := srv.Svc().Backend.CreateBucket(storageKey); err != nil {
		t.Fatal(err)
	}
	uploadID, err := srv.Svc().CreateMultipartUpload(t.Context(), storageKey, "dir/big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Svc().CreateMultipartUpload(t.Context(), storageKey, "other.bin"); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/"+logical+"?uploads", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list multipart %d: %s", resp.StatusCode, body)
	}
	text := string(body)
	if !strings.Contains(text, "ListMultipartUploadsResult") {
		t.Fatalf("expected ListMultipartUploadsResult, got %s", text)
	}
	if strings.Count(text, "<Upload>") != 2 {
		t.Fatalf("active uploads: %s", text)
	}
	if !strings.Contains(text, uploadID) || !strings.Contains(text, "dir/big.bin") {
		t.Fatalf("missing upload %s in %s", uploadID, text)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/"+logical+"?uploads&prefix=dir/", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	text = string(body)
	if strings.Count(text, "<Upload>") != 1 || !strings.Contains(text, "dir/big.bin") || strings.Contains(text, "other.bin") {
		t.Fatalf("prefix filter: %s", text)
	}
}

func TestCreateBucketObjectLockHeaderEnablesVersioning(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bucket := "locked-at-create"
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket, nil)
	req.Header.Set("X-Amz-Bucket-Object-Lock-Enabled", "true")
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create %d", resp.StatusCode)
	}
	rec, err := srv.Meta().GetBucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.ObjectLock || !rec.Versioning || rec.VersioningSuspended {
		t.Fatalf("header create lock=%v versioning=%v suspended=%v", rec.ObjectLock, rec.Versioning, rec.VersioningSuspended)
	}
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/"+bucket+"?versioning", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<Status>Enabled</Status>") || strings.Contains(string(body), "Suspended") {
		t.Fatalf("versioning status %d: %s", resp.StatusCode, body)
	}
	suspend := `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Suspended</Status></VersioningConfiguration>`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"?versioning", strings.NewReader(suspend))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "InvalidBucketState") {
		t.Fatalf("suspend locked bucket %d: %s", resp.StatusCode, body)
	}
}

func TestObjectLockReportsVersioningEnabled(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	bucket := "locked"
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket, nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create %d", resp.StatusCode)
	}

	lockXML := `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled></ObjectLockConfiguration>`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"?object-lock", strings.NewReader(lockXML))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put lock %d", resp.StatusCode)
	}

	rec, err := srv.Meta().GetBucket(bucket)
	if err != nil {
		t.Fatal(err)
	}
	if !rec.Versioning || rec.VersioningSuspended {
		t.Fatalf("enabling object lock stored versioning=%v suspended=%v", rec.Versioning, rec.VersioningSuspended)
	}
	// A row that still says Suspended must be reported as Enabled.
	rec.VersioningSuspended = true
	if err := srv.Meta().UpdateBucket(rec); err != nil {
		t.Fatal(err)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/"+bucket+"?versioning", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "<Status>Enabled</Status>") || strings.Contains(string(body), "Suspended") {
		t.Fatalf("versioning status %d: %s", resp.StatusCode, body)
	}

	suspend := `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Suspended</Status></VersioningConfiguration>`
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/"+bucket+"?versioning", strings.NewReader(suspend))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || !strings.Contains(string(body), "InvalidBucketState") {
		t.Fatalf("suspend locked bucket %d: %s", resp.StatusCode, body)
	}
}
