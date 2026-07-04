package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestFederationCreate_withExplicitLocalClusterID(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]string{
		"name":       "peer-explicit",
		"endpoint":   "http://127.0.0.1:9003",
		"cluster_id": "local",
	})
	req := authReq(http.MethodPost, "/api/v1/federation/clusters", tok, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create federation %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Cluster metadata.FederationCluster `json:"cluster"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Cluster.ClusterID != "local" {
		t.Fatalf("expected cluster_id local, got %q", resp.Cluster.ClusterID)
	}
}

func TestFederationCreate_withLocalClusterID(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]string{
		"name":     "peer-a",
		"endpoint": "http://127.0.0.1:9001",
	})
	req := authReq(http.MethodPost, "/api/v1/federation/clusters", tok, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create federation %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Cluster metadata.FederationCluster `json:"cluster"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Cluster.ClusterID != "local" {
		t.Fatalf("expected cluster_id local, got %q", resp.Cluster.ClusterID)
	}
}

func TestFederationCreate_rejectsUnknownClusterID(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	body, _ := json.Marshal(map[string]string{
		"name":       "peer-b",
		"endpoint":   "http://127.0.0.1:9002",
		"cluster_id": "does-not-exist",
	})
	req := authReq(http.MethodPost, "/api/v1/federation/clusters", tok, body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown cluster_id want 400 got %d %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown cluster_id") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestFederationList_filterByClusterID(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	for _, ep := range []string{"http://a:9000", "http://b:9000"} {
		body, _ := json.Marshal(map[string]string{
			"name": "p", "endpoint": ep, "cluster_id": "local",
		})
		req := authReq(http.MethodPost, "/api/v1/federation/clusters", tok, body)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create %d", rec.Code)
		}
	}
	req := authReq(http.MethodGet, "/api/v1/federation/clusters?cluster_id=local", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %d", rec.Code)
	}
	var resp struct {
		Clusters []metadata.FederationCluster `json:"clusters"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Clusters) != 2 {
		t.Fatalf("expected 2 filtered peers, got %d", len(resp.Clusters))
	}
}
