package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestPresignRoundTrip(t *testing.T) {
	creds := auth.Credentials{AccessKey: "testkey", SecretKey: "testsecret"}
	signer := auth.NewSigner("us-east-1", func(k string) (auth.Credentials, bool) {
		return creds, k == creds.AccessKey
	})

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := signer.Authenticate(r)
		if err != nil {
			t.Errorf("auth failed: %v host=%q path=%q query=%q", err, r.Host, r.URL.Path, r.URL.RawQuery)
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	url, err := signer.PresignURL(http.MethodPut, ts.URL, "b", "k", creds, 3600*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d url=%s", resp.StatusCode, url)
	}
}
