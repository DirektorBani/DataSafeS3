package api_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestTrustedClusterReplication_putObject(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	t.Setenv("STORAGE_TRUSTED_CLUSTER_REPL_ENABLED", "true")
	s := testServer(t)
	ctx := context.Background()

	var mu sync.Mutex
	received := map[string][]byte{}
	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			received[r.URL.Path] = body
			mu.Unlock()
			w.Header().Set("ETag", `"test-etag"`)
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer peerSrv.Close()

	clusterID := "repl-peer-1"
	now := time.Now().UTC()
	rec := metadata.TrustedCluster{
		ID: clusterID, Name: "Peer", Endpoint: peerSrv.URL,
		AuthMode: metadata.TrustedClusterAuthMTLS, Status: metadata.TrustedClusterStatusHealthy,
		CreatedAt: now, Active: true,
	}
	if err := s.Meta().PutTrustedCluster(rec); err != nil {
		t.Fatal(err)
	}
	s.ProcessTrustedClusterPeersOnce(ctx)

	srcBucket := "repl-src"
	destBucket := "repl-dst"
	if err := s.Svc().CreateBucket(ctx, srcBucket, "admin"); err != nil {
		t.Fatal(err)
	}
	rule := metadata.SiteReplicationRule{
		ID:               "rule-1",
		TrustedClusterID: clusterID,
		SourceBucket:     srcBucket,
		DestBucket:       destBucket,
		Direction:        "one-way",
		Enabled:          true,
		CreatedAt:        now,
	}
	if err := s.Meta().PutSiteReplicationRule(rule); err != nil {
		t.Fatal(err)
	}

	payload := []byte("trusted-cluster-repl-payload")
	_, err := s.Svc().PutObject(ctx, srcBucket, "probe.txt", bytes.NewReader(payload), int64(len(payload)), "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	s.EnqueueSiteReplicationForTest(metadata.SiteReplEventPut, srcBucket, "probe.txt")

	deadline := time.Now().Add(5 * time.Second)
	for {
		s.ProcessSiteReplicationOnce(ctx)
		mu.Lock()
		got := received["/"+destBucket+"/probe.txt"]
		mu.Unlock()
		if len(got) > 0 {
			if !bytes.Equal(got, payload) {
				t.Fatalf("payload mismatch: %q", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("peer did not receive object; paths: %v", keysOf(received))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestClusterRevoke_stopsReplication(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	t.Setenv("STORAGE_TRUSTED_CLUSTER_REPL_ENABLED", "true")
	s := testServer(t)
	ctx := context.Background()

	peerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer peerSrv.Close()

	clusterID := "revoke-repl-peer"
	now := time.Now().UTC()
	rec := metadata.TrustedCluster{
		ID: clusterID, Name: "Peer", Endpoint: peerSrv.URL,
		AuthMode: metadata.TrustedClusterAuthMTLS, Status: metadata.TrustedClusterStatusHealthy,
		CreatedAt: now, Active: true,
	}
	if err := s.Meta().PutTrustedCluster(rec); err != nil {
		t.Fatal(err)
	}
	s.ProcessTrustedClusterPeersOnce(ctx)

	if err := s.Svc().CreateBucket(ctx, "b1", "admin"); err != nil {
		t.Fatal(err)
	}
	body := []byte("x")
	_, err := s.Svc().PutObject(ctx, "b1", "k.txt", bytes.NewReader(body), int64(len(body)), "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}

	rule := metadata.SiteReplicationRule{
		ID: "rule-rev", TrustedClusterID: clusterID,
		SourceBucket: "b1", DestBucket: "b2", Enabled: true, CreatedAt: now,
	}
	_ = s.Meta().PutSiteReplicationRule(rule)
	s.EnqueueSiteReplicationForTest(metadata.SiteReplEventPut, "b1", "k.txt")

	tok := loginToken(t, s, "admin", "admin")
	req := clusterAuthReq(http.MethodPost, "/api/v1/clusters/"+clusterID+"/revoke", tok, nil)
	recResp := httptest.NewRecorder()
	s.Handler().ServeHTTP(recResp, req)
	if recResp.Code != http.StatusOK {
		t.Fatalf("revoke %d", recResp.Code)
	}

	s.ProcessTrustedClusterPeersOnce(ctx)
	s.ProcessSiteReplicationOnce(ctx)

	if s.TrustedClusterHealthyForTest(clusterID) {
		t.Fatal("expected unhealthy trusted cluster after revoke")
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
