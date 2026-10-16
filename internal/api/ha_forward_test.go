package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxyToLeaderPreservesHostAndBody(t *testing.T) {
	var gotHost, gotFwd, gotBody string
	leader := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		gotFwd = r.Header.Get("X-DataSafe-Forwarded")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("x-amz-version-id", "v1")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(leader.Close)

	req := httptest.NewRequest(http.MethodPut, "http://follower.example/bucket/key?x=1", strings.NewReader("payload"))
	req.Host = "datasafe.example:9000"
	rr := httptest.NewRecorder()
	if !proxyToLeader(rr, req, leader.URL) {
		t.Fatal("forward failed")
	}
	if gotHost != "datasafe.example:9000" {
		t.Fatalf("host %q", gotHost)
	}
	if gotFwd != "1" {
		t.Fatalf("forwarded %q", gotFwd)
	}
	if gotBody != "payload" {
		t.Fatalf("body %q", gotBody)
	}
	if rr.Code != http.StatusOK || rr.Header().Get("x-amz-version-id") != "v1" {
		t.Fatalf("response %d %#v", rr.Code, rr.Header())
	}
}
