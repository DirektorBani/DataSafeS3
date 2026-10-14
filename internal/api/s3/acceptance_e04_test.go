package s3_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/auth"
)

func TestUnsignedHeaderIsNotStored(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	put(t, ts.URL, creds, "/bucket", "")

	req, err := http.NewRequest(http.MethodPut, ts.URL+"/bucket/evil.txt", strings.NewReader("nope"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	signRequest(t, req, creds, "us-east-1")
	req.Header.Set("x-amz-meta-evil", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "not signed") {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	req, _ = http.NewRequest(http.MethodHead, ts.URL+"/bucket/evil.txt", nil)
	req.Host = req.URL.Host
	signRequest(t, req, creds, "us-east-1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("unsigned object was stored, meta %q", resp.Header.Get("x-amz-meta-evil"))
	}
}

func TestListObjectsPages(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	put(t, ts.URL, creds, "/paged", "")
	for _, key := range []string{"a", "b", "c"} {
		put(t, ts.URL, creds, "/paged/"+key, key)
	}
	v1 := getRaw(t, ts.URL, creds, "/paged?max-keys=1")
	if !strings.Contains(v1, "<IsTruncated>true</IsTruncated>") || !strings.Contains(v1, "<NextMarker>") {
		t.Fatalf("v1 page: %s", v1)
	}
	v2 := getRaw(t, ts.URL, creds, "/paged?list-type=2&max-keys=1")
	if !strings.Contains(v2, "<IsTruncated>true</IsTruncated>") || !strings.Contains(v2, "<NextContinuationToken>") {
		t.Fatalf("v2 page: %s", v2)
	}
	token := between(t, v2, "<NextContinuationToken>", "</NextContinuationToken>")
	page2 := getRaw(t, ts.URL, creds, "/paged?list-type=2&max-keys=1&continuation-token="+token)
	if strings.Contains(page2, "<Key>"+token+"</Key>") {
		t.Fatalf("continuation repeated %s\n%s", token, page2)
	}
	if !strings.Contains(page2, "<Key>") {
		t.Fatalf("empty second page: %s", page2)
	}
}

func TestListObjectVersionsPages(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	put(t, ts.URL, creds, "/vers", "")
	put(t, ts.URL, creds, "/vers?versioning", `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
	put(t, ts.URL, creds, "/vers/k", "one")
	put(t, ts.URL, creds, "/vers/k", "two")
	body := getRaw(t, ts.URL, creds, "/vers?versions&max-keys=1")
	if !strings.Contains(body, "<IsTruncated>true</IsTruncated>") || !strings.Contains(body, "<NextKeyMarker>") || !strings.Contains(body, "<NextVersionIdMarker>") {
		t.Fatalf("versions page: %s", body)
	}
}

func TestLifecycleFilterRoundTripAndReject(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	put(t, ts.URL, creds, "/life", "")
	good := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>keep</ID><Filter><Prefix>logs/</Prefix></Filter><Status>Enabled</Status><Expiration><Days>30</Days></Expiration></Rule></LifecycleConfiguration>`
	put(t, ts.URL, creds, "/life?lifecycle", good)
	got := getRaw(t, ts.URL, creds, "/life?lifecycle")
	if !strings.Contains(got, "logs/") || !strings.Contains(got, "<Days>30</Days>") {
		t.Fatalf("round trip: %s", got)
	}
	bad := `<LifecycleConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Rule><ID>nc</ID><Filter><Prefix>logs/</Prefix></Filter><Status>Enabled</Status><NoncurrentVersionExpiration><NoncurrentDays>1</NoncurrentDays></NoncurrentVersionExpiration></Rule></LifecycleConfiguration>`
	status := putStatus(t, ts.URL, creds, "/life?lifecycle", bad)
	if status != http.StatusBadRequest {
		t.Fatalf("noncurrent status %d", status)
	}
	got = getRaw(t, ts.URL, creds, "/life?lifecycle")
	if !strings.Contains(got, "logs/") || strings.Contains(got, "<Days>0</Days>") {
		t.Fatalf("rejected rule changed the stored prefix rule: %s", got)
	}
}

func TestObjectRetentionModeSurvivesBucketDefault(t *testing.T) {
	srv, creds := testServer(t)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	put(t, ts.URL, creds, "/lock", "")
	put(t, ts.URL, creds, "/lock?object-lock", `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled><Rule><DefaultRetention><Mode>GOVERNANCE</Mode><Days>1</Days></DefaultRetention></Rule></ObjectLockConfiguration>`)
	put(t, ts.URL, creds, "/lock/file.txt", "secret")
	put(t, ts.URL, creds, "/lock/file.txt?retention", `<Retention xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Mode>COMPLIANCE</Mode><RetainUntilDate>2030-01-02T03:04:05Z</RetainUntilDate></Retention>`)
	put(t, ts.URL, creds, "/lock?object-lock", `<ObjectLockConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><ObjectLockEnabled>Enabled</ObjectLockEnabled><Rule><DefaultRetention><Mode>GOVERNANCE</Mode><Days>1</Days></DefaultRetention></Rule></ObjectLockConfiguration>`)
	got := getRaw(t, ts.URL, creds, "/lock/file.txt?retention")
	if !strings.Contains(got, "COMPLIANCE") || !strings.Contains(got, "2030-01-02") {
		t.Fatalf("retention %s", got)
	}
}

func put(t *testing.T, base string, creds auth.Credentials, path, body string) {
	t.Helper()
	if putStatus(t, base, creds, path, body) >= 300 {
		t.Fatalf("PUT %s failed", path)
	}
}

func putStatus(t *testing.T, base string, creds auth.Credentials, path, body string) int {
	t.Helper()
	var rdr *strings.Reader
	if body != "" || strings.Contains(path, "") {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPut, base+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Logf("PUT %s %d %s", path, resp.StatusCode, raw)
	}
	return resp.StatusCode
}

func getRaw(t *testing.T, base string, creds auth.Credentials, path string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = req.URL.Host
	signRequest(t, req, creds, "us-east-1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		t.Fatalf("GET %s %d %s", path, resp.StatusCode, raw)
	}
	return string(raw)
}

func between(t *testing.T, s, a, b string) string {
	t.Helper()
	i := strings.Index(s, a)
	if i < 0 {
		t.Fatalf("missing %s in %s", a, s)
	}
	s = s[i+len(a):]
	j := strings.Index(s, b)
	if j < 0 {
		t.Fatalf("missing %s", b)
	}
	return s[:j]
}
