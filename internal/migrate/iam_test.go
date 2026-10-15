package migrate_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/migrate"
	"github.com/DirektorBani/datasafe/internal/policy"
)

func TestDecryptAdminRoundTrip(t *testing.T) {
	plain := []byte(`{"users":{"u00":{"status":"enabled"}}}`)
	sealed, err := migrate.EncryptAdmin("minioadmin", plain)
	if err != nil {
		t.Fatal(err)
	}
	got, err := migrate.DecryptAdmin("minioadmin", sealed)
	if err != nil || string(got) != string(plain) {
		t.Fatalf("decrypt %s %v", got, err)
	}
	asJSON, err := migrate.DecryptAdmin("minioadmin", plain)
	if err != nil || string(asJSON) != string(plain) {
		t.Fatalf("plaintext %s %v", asJSON, err)
	}
}

func TestMapBucketPoliciesWarnsInsteadOfDroppingQuietly(t *testing.T) {
	docs := map[string]string{
		"ok":  `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::photos"]}]}`,
		"bad": `{"Version":"2012-10-17","Statement":[{"Sid":"pref","Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::photos/*","Condition":{"StringEquals":{"s3:prefix":"a"}}}]}`,
		"all": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":"s3:*","Resource":"*"}]}`,
	}
	who := map[string][]string{
		"ok":  {policy.UserARN("u00")},
		"bad": {policy.UserARN("u00")},
		"all": {policy.UserARN("u00")},
	}
	mapped, warnings := migrate.MapBucketPolicies(docs, who)
	if len(mapped["photos"]) != 1 {
		t.Fatalf("photos statements %+v", mapped)
	}
	text := strings.Join(warnings, "\n")
	if !strings.Contains(text, "s3:prefix") || !strings.Contains(text, "every bucket") {
		t.Fatalf("warnings %s", text)
	}
	raw, _ := json.Marshal(mapped["photos"])
	if strings.Contains(string(raw), "s3:prefix") {
		t.Fatalf("unsupported condition was copied: %s", raw)
	}
}

func TestE05IAMImportTwentyUsers(t *testing.T) {
	t.Setenv("STORAGE_RATE_LIMIT_LOGIN", "1000")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NATS_URL", "")
	t.Setenv("STORAGE_KMS_PROVIDER", "")
	t.Setenv("STORAGE_KMS_VAULT_ADDR", "")
	s, err := api.NewServer(api.Config{
		DataDir: t.TempDir(), AdminUser: "admin", AdminPassword: "admin",
		JWTSecret: "test", SSEMasterKey: "test-sse-master-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cfg, err := s.Meta().GetSystemConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.InitialSetupCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	ds := httptest.NewServer(s.Handler())
	t.Cleanup(ds.Close)
	admin := loginIAM(t, ds.URL, "admin", "admin")
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest(http.MethodPost, ds.URL+"/api/v1/buckets/b"+fmt.Sprint(i), strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+admin)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("bucket %d %d %s", i, resp.StatusCode, body)
		}
	}
	src := startMinIOIAM(t)
	dir := t.TempDir()
	rep, err := migrate.RunIAM(t.Context(), migrate.IAMOptions{
		Source:        src,
		Targets:       []migrate.Endpoint{{URL: ds.URL, Region: "us-east-1"}},
		AdminUser:     "admin",
		AdminPassword: "admin",
		KeysFile:      filepath.Join(dir, "keys.json"),
		ReportDir:     filepath.Join(dir, "report"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.UsersTransferred != 20 || rep.GroupsTransferred != 5 || rep.PoliciesTransferred != 10 {
		t.Fatalf("counts %+v warnings %v errors %v", rep, rep.Warnings, rep.Errors)
	}
	for _, name := range []string{"users", "groups", "policies", "access"} {
		if rep.Verdicts[name] != "PASS" {
			t.Fatalf("verdict %s %+v probes %+v warnings %v", name, rep.Verdicts, rep.Probes, rep.Warnings)
		}
	}
	if len(rep.Probes) < 20 {
		t.Fatalf("probes %d", len(rep.Probes))
	}
	for _, p := range rep.Probes {
		if p.Verdict != "PASS" || p.Status != http.StatusOK {
			t.Fatalf("probe %+v", p)
		}
	}
	warn := strings.Join(rep.Warnings, "\n")
	if !strings.Contains(warn, "ldap-user") || !strings.Contains(warn, "s3:prefix") || !strings.Contains(warn, "service account") {
		t.Fatalf("missing warnings %s", warn)
	}
	keysRaw, err := os.ReadFile(filepath.Join(dir, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(keysRaw), "MINIO-SECRET-SHOULD-NOT-LEAK") {
		t.Fatal("minio secret was copied")
	}
	info, err := os.Stat(filepath.Join(dir, "keys.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("keys mode %o", info.Mode().Perm())
	}
	var keys []struct {
		Username  string `json:"username"`
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal(keysRaw, &keys); err != nil || len(keys) < 20 {
		t.Fatalf("keys %d %v", len(keys), err)
	}
	users := listIAMUsers(t, ds.URL, admin)
	if len(users) < 21 {
		t.Fatalf("users %d", len(users))
	}
	pol := getIAMPolicy(t, ds.URL, admin, "b0")
	if !strings.Contains(pol, "arn:aws:iam::datasafe:access-key/") || strings.Contains(pol, "s3:prefix") {
		t.Fatalf("policy %s", pol)
	}
	if _, err := os.Stat(filepath.Join(dir, "report", "summary.md")); err != nil {
		t.Fatal(err)
	}
}

func TestIAMWritesFollowTheLeader(t *testing.T) {
	var mu sync.Mutex
	leaderReady := false
	var followerUsers, leaderUsers int
	follower := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte(`{"is_leader":false}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"not cluster leader"}`))
		mu.Lock()
		followerUsers++
		leaderReady = true
		mu.Unlock()
	}))
	t.Cleanup(follower.Close)
	leaderSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/healthz" {
			mu.Lock()
			ready := leaderReady
			mu.Unlock()
			fmt.Fprintf(w, `{"is_leader":%t}`, ready)
			return
		}
		mu.Lock()
		leaderUsers++
		mu.Unlock()
		switch {
		case r.URL.Path == "/api/v1/admin/login":
			_, _ = w.Write([]byte(`{"token":"t"}`))
		case r.URL.Path == "/api/v1/users":
			_, _ = w.Write([]byte(`{"id":"u1","user_id":"u1","username":"alice"}`))
		case r.URL.Path == "/api/v1/keys":
			_, _ = w.Write([]byte(`{"access_key":"AKIATEST","secret_key":"secretsecretsecret"}`))
		case strings.Contains(r.URL.Path, "/access-keys"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"access_key":"AKIATEST"}`))
		case strings.Contains(r.URL.Path, "/policy"):
			_, _ = w.Write([]byte(`{"policy":""}`))
		case strings.Contains(r.URL.Path, "/access"):
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/box" || strings.HasPrefix(r.URL.Path, "/box"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<ListBucketResult></ListBucketResult>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(leaderSrv.Close)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte(`{}`)
		switch {
		case strings.HasSuffix(r.URL.Path, "/list-users"):
			body = []byte(`{"alice":{"status":"enabled","policyName":"read"}}`)
		case strings.HasSuffix(r.URL.Path, "/groups"):
			body = []byte(`[]`)
		case strings.HasSuffix(r.URL.Path, "/list-canned-policies"):
			body = []byte(`{"read":"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:ListBucket\"],\"Resource\":[\"arn:aws:s3:::box\"]}]}"}`)
		case strings.Contains(r.URL.Path, "/idp/"):
			w.WriteHeader(http.StatusNotFound)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sealed, err := migrate.EncryptAdmin("minioadmin", body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(sealed)
	}))
	t.Cleanup(src.Close)
	dir := t.TempDir()
	rep, err := migrate.RunIAM(t.Context(), migrate.IAMOptions{
		Source:        migrate.Endpoint{URL: src.URL, AccessKey: "minioadmin", SecretKey: "minioadmin", Region: "us-east-1"},
		Targets:       []migrate.Endpoint{{URL: follower.URL, Region: "us-east-1"}, {URL: leaderSrv.URL, Region: "us-east-1"}},
		AdminUser:     "admin",
		AdminPassword: "admin",
		KeysFile:      filepath.Join(dir, "keys.json"),
		ReportDir:     filepath.Join(dir, "report"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.UsersTransferred != 1 || rep.Verdicts["access"] != "PASS" {
		t.Fatalf("report %+v err warnings %v", rep, rep.Warnings)
	}
	mu.Lock()
	defer mu.Unlock()
	if leaderUsers < 1 || followerUsers < 1 {
		t.Fatalf("leader writes %d follower writes %d", leaderUsers, followerUsers)
	}
}

func TestMergeBucketPolicyIsByteStable(t *testing.T) {
	add := []policy.Statement{{
		Effect:    "Allow",
		Action:    []string{"s3:GetObject", "s3:ListBucket"},
		Resource:  []string{"arn:aws:s3:::bkt2/home/*", "arn:aws:s3:::bkt2"},
		Principal: json.RawMessage(`{"AWS":"arn:aws:iam::datasafe:access-key/AKIA"}`),
		Condition: map[string]map[string]any{"IpAddress": {"aws:SourceIp": "10.1.0.0/16"}},
	}}
	first, err := migrate.MergeBucketPolicy("", add)
	if err != nil {
		t.Fatal(err)
	}
	reversed := []policy.Statement{{
		Effect:    "allow",
		Action:    []string{"s3:ListBucket", "s3:GetObject"},
		Resource:  []string{"arn:aws:s3:::bkt2", "arn:aws:s3:::bkt2/home/*"},
		Principal: json.RawMessage(`{"AWS":["arn:aws:iam::datasafe:access-key/AKIA"]}`),
		Condition: map[string]map[string]any{"ipaddress": {"aws:SourceIp": []any{"10.1.0.0/16"}}},
	}}
	second, err := migrate.MergeBucketPolicy(first, reversed)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("rerun changed policy\n%s\n%s", first, second)
	}
	if strings.Count(second, "s3:GetObject") != 1 {
		t.Fatalf("statement duplicated: %s", second)
	}
	third, err := migrate.MergeBucketPolicy(second, add)
	if err != nil || third != second {
		t.Fatalf("third %s err %v", third, err)
	}
}

func TestIAMReconnectsAfterLeaderDrops(t *testing.T) {
	var mu sync.Mutex
	phase := 0
	var policyHits int
	serve := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/admin/login":
			_, _ = w.Write([]byte(`{"token":"t"}`))
		case r.URL.Path == "/api/v1/users" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"id":"u1","user_id":"u1"}`))
		case r.URL.Path == "/api/v1/users":
			_, _ = w.Write([]byte(`{"users":[{"id":"u1","username":"alice"}]}`))
		case strings.Contains(r.URL.Path, "/access-keys"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"access_key":"AKIATEST"}`))
			mu.Lock()
			phase = 1
			mu.Unlock()
		case strings.Contains(r.URL.Path, "/policy") && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"policy":""}`))
		case strings.Contains(r.URL.Path, "/policy"):
			_, _ = w.Write([]byte(`{"ok":true}`))
		case strings.HasPrefix(r.URL.Path, "/box"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<ListBucketResult></ListBucketResult>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		p := phase
		mu.Unlock()
		if r.URL.Path == "/healthz" {
			fmt.Fprintf(w, `{"is_leader":%t}`, p == 0)
			return
		}
		if p == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				conn.Close()
			}
			return
		}
		serve(w, r)
	}))
	t.Cleanup(primary.Close)
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		p := phase
		if strings.Contains(r.URL.Path, "/policy") {
			policyHits++
		}
		mu.Unlock()
		if r.URL.Path == "/healthz" {
			fmt.Fprintf(w, `{"is_leader":%t}`, p == 1)
			return
		}
		serve(w, r)
	}))
	t.Cleanup(secondary.Close)
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := []byte(`{}`)
		switch {
		case strings.HasSuffix(r.URL.Path, "/list-users"):
			body = []byte(`{"alice":{"status":"enabled","policyName":"read"}}`)
		case strings.HasSuffix(r.URL.Path, "/groups"):
			body = []byte(`[]`)
		case strings.HasSuffix(r.URL.Path, "/list-canned-policies"):
			body = []byte(`{"read":"{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Allow\",\"Action\":[\"s3:ListBucket\"],\"Resource\":[\"arn:aws:s3:::box\"]}]}"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sealed, err := migrate.EncryptAdmin("minioadmin", body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(sealed)
	}))
	t.Cleanup(src.Close)
	dir := t.TempDir()
	keysPath := filepath.Join(dir, "keys.json")
	rep, err := migrate.RunIAM(t.Context(), migrate.IAMOptions{
		Source:        migrate.Endpoint{URL: src.URL, AccessKey: "minioadmin", SecretKey: "minioadmin", Region: "us-east-1"},
		Targets:       []migrate.Endpoint{{URL: primary.URL, Region: "us-east-1"}, {URL: secondary.URL, Region: "us-east-1"}},
		AdminUser:     "admin",
		AdminPassword: "admin",
		KeysFile:      keysPath,
		ReportDir:     filepath.Join(dir, "report"),
	})
	if err != nil {
		t.Fatalf("import: %v errors %v", err, rep.Errors)
	}
	raw, err := os.ReadFile(keysPath)
	if err != nil || !strings.Contains(string(raw), `"username": "alice"`) || !strings.Contains(string(raw), `"secret_key":`) {
		t.Fatalf("keys file %s err %v", raw, err)
	}
	mu.Lock()
	hits := policyHits
	mu.Unlock()
	if hits < 1 {
		t.Fatal("import did not continue on the new leader")
	}
}

func TestFromMinIOLiveIAM(t *testing.T) {
	ep := os.Getenv("MINIO_ENDPOINT")
	key := os.Getenv("MINIO_ACCESS_KEY")
	secret := os.Getenv("MINIO_SECRET_KEY")
	if ep == "" || key == "" || secret == "" {
		t.Skip("MINIO_ENDPOINT is not set")
	}
	_, err := migrate.ReadMinIOIAM(t.Context(), migrate.Endpoint{
		URL: ep, AccessKey: key, SecretKey: secret, Region: "us-east-1",
	})
	if err != nil {
		t.Skipf("minio admin api: %v", err)
	}
}

func startMinIOIAM(t *testing.T) migrate.Endpoint {
	t.Helper()
	users := map[string]any{}
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("u%02d", i)
		users[name] = map[string]any{
			"status":     "enabled",
			"policyName": fmt.Sprintf("p%d,pbad", i%10),
			"memberOf":   []string{fmt.Sprintf("g%d", i/4)},
			"secretKey":  "MINIO-SECRET-SHOULD-NOT-LEAK",
		}
	}
	groups := make([]string, 5)
	for i := 0; i < 5; i++ {
		groups[i] = fmt.Sprintf("g%d", i)
	}
	policies := map[string]string{}
	for i := 0; i < 10; i++ {
		policies[fmt.Sprintf("p%d", i)] = fmt.Sprintf(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject"],"Resource":["arn:aws:s3:::b%d","arn:aws:s3:::b%d/*"]}]}`, i, i)
	}
	policies["pbad"] = `{"Version":"2012-10-17","Statement":[{"Sid":"pref","Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::b0/*","Condition":{"StringEquals":{"s3:prefix":"a"}}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		switch {
		case strings.Contains(r.URL.Path, "/idp/ldap/"):
			body = []byte(`["ldap-user"]`)
		case strings.Contains(r.URL.Path, "/idp/"):
			w.WriteHeader(http.StatusNotFound)
			return
		case strings.HasSuffix(r.URL.Path, "/list-users"):
			body, _ = json.Marshal(users)
		case strings.HasSuffix(r.URL.Path, "/groups"):
			body, _ = json.Marshal(groups)
		case strings.HasSuffix(r.URL.Path, "/group-desc"):
			w.WriteHeader(http.StatusUpgradeRequired)
			return
		case strings.HasSuffix(r.URL.Path, "/group"):
			g := r.URL.Query().Get("group")
			var n int
			fmt.Sscanf(g, "g%d", &n)
			members := []string{}
			for i := n * 4; i < n*4+4 && i < 20; i++ {
				members = append(members, fmt.Sprintf("u%02d", i))
			}
			body, _ = json.Marshal(map[string]any{"members": members, "policy": fmt.Sprintf("p%d", n), "status": "enabled"})
		case strings.HasSuffix(r.URL.Path, "/list-canned-policies"):
			raw := map[string]json.RawMessage{}
			for name, doc := range policies {
				raw[name] = json.RawMessage(doc)
			}
			body, _ = json.Marshal(raw)
		case strings.HasSuffix(r.URL.Path, "/list-service-accounts"):
			if r.URL.Query().Get("user") == "u00" {
				body = []byte(`{"accounts":[{"accessKey":"SAU00"}]}`)
			} else {
				body = []byte(`{"accounts":[]}`)
			}
		case strings.HasSuffix(r.URL.Path, "/info-service-account"):
			body = []byte(`{"parentUser":"u00","impliedPolicy":true,"accountStatus":"enabled"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		sealed, err := migrate.EncryptAdmin("minioadmin", body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(sealed)
	}))
	t.Cleanup(srv.Close)
	return migrate.Endpoint{URL: srv.URL, AccessKey: "minioadmin", SecretKey: "minioadmin", Region: "us-east-1"}
}

func loginIAM(t *testing.T, base, user, pass string) string {
	t.Helper()
	resp, err := http.Post(base+"/api/v1/admin/login", "application/json", strings.NewReader(`{"username":"`+user+`","password":"`+pass+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var box struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&box); err != nil || box.Token == "" {
		t.Fatalf("login %v", err)
	}
	return box.Token
}

func listIAMUsers(t *testing.T, base, token string) []metadata.UserRecord {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var box struct {
		Users []metadata.UserRecord `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&box); err != nil {
		t.Fatal(err)
	}
	return box.Users
}

func getIAMPolicy(t *testing.T, base, token, bucket string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/buckets/"+bucket+"/policy", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("policy %d %s", resp.StatusCode, b)
	}
	return string(b)
}
