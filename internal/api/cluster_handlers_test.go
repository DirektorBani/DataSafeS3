package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/cluster/pki"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func clusterAuthReq(method, path, token string, body []byte) *http.Request {
	req := authReq(method, path, token, body)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func createPairingCode(t *testing.T, s *api.Server, tok string) string {
	t.Helper()
	req := clusterAuthReq(http.MethodPost, "/api/v1/clusters/pairing-codes", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("pairing code status %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !strings.HasPrefix(resp.Token, metadata.PairingTokenPrefix) {
		t.Fatalf("expected dsjoin token, got %q", resp.Token)
	}
	return resp.Token
}

func testServerFromFresh(t *testing.T, s *api.Server) *api.Server {
	t.Helper()
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	cfg.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestClusterPairing_replayUsedToken(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	initiator := testServer(t)
	initTok := loginToken(t, initiator, "admin", "admin")
	pairTok := createPairingCode(t, initiator, initTok)
	initiatorTS := httptest.NewServer(initiator.Handler())
	defer initiatorTS.Close()

	joiner := testServerFromFresh(t, freshTestServer(t))
	joinerTok := loginToken(t, joiner, "admin", "admin")

	joinBody, _ := json.Marshal(map[string]string{
		"initiator_url": initiatorTS.URL,
		"token":         pairTok,
		"name":          "joiner-a",
	})
	req := clusterAuthReq(http.MethodPost, "/api/v1/clusters/pair/join", joinerTok, joinBody)
	rec := httptest.NewRecorder()
	joiner.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first join %d %s", rec.Code, rec.Body.String())
	}

	joinBody2, _ := json.Marshal(map[string]string{
		"initiator_url": initiatorTS.URL,
		"token":         pairTok,
		"name":          "joiner-b",
	})
	req = clusterAuthReq(http.MethodPost, "/api/v1/clusters/pair/join", joinerTok, joinBody2)
	rec = httptest.NewRecorder()
	joiner.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized && rec.Code != http.StatusBadGateway {
		t.Fatalf("replay join want 401/502 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestClusterPairing_rejectsWrongCA(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	pairTok := createPairingCode(t, s, tok)

	// Use a leaf cert (signed client cert) as fake CA — MITM simulation.
	pkiMgr := pki.NewManager(t.TempDir())
	csrPEM, _, err := pkiMgr.GenerateCSR("evil")
	if err != nil {
		t.Fatal(err)
	}
	certPEM, _, _, _, err := pkiMgr.SignCSR(csrPEM, metadata.ClusterCertTTL)
	if err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]string{
		"token":           pairTok,
		"joiner_ca_pem":   string(certPEM),
		"joiner_csr_pem":  string(csrPEM),
		"joiner_endpoint": "http://evil.example:9000",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/pair/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("wrong CA want 400 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestClusterPairing_mTLSRequired(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	pairTok := createPairingCode(t, s, tok)

	body, _ := json.Marshal(map[string]string{
		"token":          pairTok,
		"joiner_ca_pem":  "not-a-pem",
		"joiner_csr_pem": "not-a-csr",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/clusters/pair/complete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid cert material want 400 got %d", rec.Code)
	}

	// Missing CSR/CA entirely
	body2, _ := json.Marshal(map[string]string{"token": pairTok})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/clusters/pair/complete", bytes.NewReader(body2))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing fields want 400 got %d", rec.Code)
	}
}

func TestTrustedClusterMetadata_noPlaintextSecrets(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	raw := createPairingCode(t, s, tok)
	hash := metadata.HashPairingToken(raw)
	stored, err := s.Meta().GetClusterPairingSession(hash)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TokenHash != hash {
		t.Fatalf("expected hash stored")
	}
	if strings.Contains(stored.TokenHash, metadata.PairingTokenPrefix) {
		t.Fatal("plaintext token prefix in stored hash")
	}
	if stored.TokenHash == raw {
		t.Fatal("plaintext token stored in metadata")
	}
}

func TestListTrustedClusters_includesLocal(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	req := clusterAuthReq(http.MethodGet, "/api/v1/clusters", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list clusters %d", rec.Code)
	}
	var resp struct {
		Clusters []metadata.TrustedCluster `json:"clusters"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Clusters) < 1 || !resp.Clusters[0].IsLocal {
		t.Fatalf("expected local cluster in list: %+v", resp.Clusters)
	}
}

func TestHashPairingToken_unit(t *testing.T) {
	a := metadata.HashPairingToken("dsjoin_abc")
	b := metadata.HashPairingToken("dsjoin_abc")
	if a != b || a == "" || strings.Contains(a, "dsjoin") {
		t.Fatalf("bad hash: %q", a)
	}
}

func TestClusterPairing_happyPath(t *testing.T) {
	t.Setenv("STORAGE_DEV", "true")
	initiator := testServer(t)
	initTok := loginToken(t, initiator, "admin", "admin")
	pairTok := createPairingCode(t, initiator, initTok)
	initiatorTS := httptest.NewServer(initiator.Handler())
	defer initiatorTS.Close()

	joiner := testServerFromFresh(t, freshTestServer(t))
	joinerTok := loginToken(t, joiner, "admin", "admin")

	joinBody, _ := json.Marshal(map[string]string{
		"initiator_url": initiatorTS.URL,
		"token":         pairTok,
		"name":          "remote-site",
	})
	req := clusterAuthReq(http.MethodPost, "/api/v1/clusters/pair/join", joinerTok, joinBody)
	rec := httptest.NewRecorder()
	joiner.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("join %d %s", rec.Code, rec.Body.String())
	}

	countRemote := func(t *testing.T, s *api.Server, tok string) int {
		t.Helper()
		req := clusterAuthReq(http.MethodGet, "/api/v1/clusters", tok, nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("list clusters %d", rec.Code)
		}
		var resp struct {
			Clusters []metadata.TrustedCluster `json:"clusters"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		n := 0
		for _, c := range resp.Clusters {
			if !c.IsLocal && c.Active {
				n++
			}
		}
		return n
	}
	if countRemote(t, initiator, initTok) < 1 {
		t.Fatal("initiator missing remote trusted cluster after pairing")
	}
	if countRemote(t, joiner, joinerTok) < 1 {
		t.Fatal("joiner missing remote trusted cluster after pairing")
	}
}

func TestRevokeTrustedCluster_rejectsLocal(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	listReq := clusterAuthReq(http.MethodGet, "/api/v1/clusters", tok, nil)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	var listResp struct {
		Clusters []metadata.TrustedCluster `json:"clusters"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &listResp)
	var localID string
	for _, c := range listResp.Clusters {
		if c.IsLocal {
			localID = c.ID
			break
		}
	}
	if localID == "" {
		t.Fatal("no local cluster in list")
	}
	req := clusterAuthReq(http.MethodPost, "/api/v1/clusters/"+localID+"/revoke", tok, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("revoke local want 400 got %d %s", rec.Code, rec.Body.String())
	}
}

func TestClusterReplicationRulesAPI(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	clusterID := "rules-peer-1"
	now := time.Now().UTC()
	if err := s.Meta().PutTrustedCluster(metadata.TrustedCluster{
		ID: clusterID, Name: "Peer", Endpoint: "http://127.0.0.1:9001",
		AuthMode: metadata.TrustedClusterAuthMTLS, Status: metadata.TrustedClusterStatusHealthy,
		CreatedAt: now, Active: true,
	}); err != nil {
		t.Fatal(err)
	}

	listReq := clusterAuthReq(http.MethodGet, "/api/v1/clusters/"+clusterID+"/replication-rules", tok, nil)
	listRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list rules %d %s", listRec.Code, listRec.Body.String())
	}

	body, _ := json.Marshal(map[string]string{
		"source_bucket": "src-bucket",
		"dest_bucket":   "dst-bucket",
		"direction":     "one-way",
	})
	createReq := clusterAuthReq(http.MethodPost, "/api/v1/clusters/"+clusterID+"/replication-rules", tok, body)
	createRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create rule %d %s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Rule metadata.SiteReplicationRule `json:"rule"`
	}
	_ = json.Unmarshal(createRec.Body.Bytes(), &createResp)
	if createResp.Rule.ID == "" {
		t.Fatal("missing rule id")
	}

	delReq := clusterAuthReq(http.MethodDelete, "/api/v1/clusters/"+clusterID+"/replication-rules/"+createResp.Rule.ID, tok, nil)
	delRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("delete rule %d %s", delRec.Code, delRec.Body.String())
	}
}
