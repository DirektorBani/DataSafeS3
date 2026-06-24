package s3_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLifecycleExpiration(t *testing.T) {
	srv, creds := testServer(t)
	ctx := context.Background()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	setupBucket(t, ts, creds, "life")

	token, err := srv.AdminToken()
	if err != nil {
		t.Fatal(err)
	}
	lifecycleBody := `{"rules":[{"id":"expire","prefix":"","expiration_days":1,"enabled":true}]}`
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/buckets/life/lifecycle", bytes.NewReader([]byte(lifecycleBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set lifecycle %d", resp.StatusCode)
	}

	putReq, _ := http.NewRequest(http.MethodPut, ts.URL+"/life/old.txt", bytes.NewReader([]byte("x")))
	signRequest(t, putReq, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(putReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if err := srv.SetObjectModified("life", "old.txt", time.Now().UTC().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	srv.PruneLifecycleOnce(ctx)

	getReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/life/old.txt", nil)
	signRequest(t, getReq, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected expired object gone, status %d", resp.StatusCode)
	}
}
