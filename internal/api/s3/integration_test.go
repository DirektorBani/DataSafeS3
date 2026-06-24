package s3_test

import (
	"bytes"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
)

func setupBucket(t *testing.T, ts *httptest.Server, creds auth.Credentials, bucket string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/"+bucket, nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestMultipartUpload(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	setupBucket(t, ts, creds, "mp")

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/mp/large.bin?uploads", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create multipart %d: %s", resp.StatusCode, body)
	}
	var initResp struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(body, &initResp); err != nil || initResp.UploadID == "" {
		t.Fatalf("upload id: %v body %s", err, body)
	}

	p1 := []byte("aaa")
	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/mp/large.bin?partNumber=1&uploadId="+initResp.UploadID, bytes.NewReader(p1))
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload part %d", resp.StatusCode)
	}

	completeBody := `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>"` +
		strings.Trim(resp.Header.Get("ETag"), `"`) + `"</ETag></Part></CompleteMultipartUpload>`
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/mp/large.bin?uploadId="+initResp.UploadID, strings.NewReader(completeBody))
	req.Header.Set("Content-Type", "application/xml")
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete multipart %d", resp.StatusCode)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/mp/large.bin", nil)
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "aaa" {
		t.Fatalf("got %q", got)
	}
}

func TestPresignedURL(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	setupBucket(t, ts, creds, "ps")

	signer := auth.NewSigner("us-east-1", func(k string) (auth.Credentials, bool) {
		return creds, k == creds.AccessKey
	})
	url, err := signer.PresignURL(http.MethodPut, ts.URL, "ps", "signed.txt", creds, 3600*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPut, url, bytes.NewReader([]byte("presigned")))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned put %d", resp.StatusCode)
	}
}

func TestRangeRequest(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	setupBucket(t, ts, creds, "rg")

	payload := []byte("0123456789")
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/rg/range.bin", bytes.NewReader(payload))
	signRequest(t, req, creds, "us-east-1")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/rg/range.bin", nil)
	req.Header.Set("Range", "bytes=2-5")
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status %d", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	if string(got) != "2345" {
		t.Fatalf("got %q", got)
	}
}

func init() {
	_ = api.Config{}
}
