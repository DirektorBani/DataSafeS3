package migrate_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/migrate"
	"github.com/DirektorBani/datasafe/internal/policy"
)

func TestDenyWithUnsupportedConditionStays(t *testing.T) {
	docs := map[string]string{
		"d": `{"Version":"2012-10-17","Statement":[{"Sid":"time","Effect":"Deny","Action":"s3:GetObject","Resource":"arn:aws:s3:::bkt1/*","Condition":{"DateGreaterThan":{"aws:CurrentTime":"2000-01-01T00:00:00Z"}}},{"Effect":"Allow","Action":"s3:GetObject","Resource":"arn:aws:s3:::bkt1/*"}]}`,
	}
	mapped, warnings := migrate.MapBucketPolicies(docs, map[string][]string{"d": {policy.AccessKeyARN("AKIA")}})
	raw, _ := json.Marshal(mapped["bkt1"])
	if !strings.Contains(string(raw), "Deny") || strings.Contains(string(raw), "CurrentTime") {
		t.Fatalf("policy %s warnings %v", raw, warnings)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "unconditionally") {
		t.Fatalf("warnings %v", warnings)
	}
}

func TestIAMRightsAreNotBroaderAndRerunKeepsState(t *testing.T) {
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
	req, _ := http.NewRequest(http.MethodPost, ds.URL+"/api/v1/buckets/bkt1", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("bucket %d", resp.StatusCode)
	}
	keeperID := createIAMUser(t, ds.URL, admin, "keeper")
	keeperKey, keeperSecret := importIAMKey(t, ds.URL, admin, keeperID, "AKIASAVEKEEPER1", "keepersecretkeeper")
	putGrant(t, ds.URL, admin, "bkt1", keeperID, true)
	for _, key := range []string{"pub/a", "priv/a", "home/a"} {
		putObject(t, ds.URL, "datasafe", "datasafesecret", "bkt1/"+key, key)
	}
	src := startRightsMinIO(t)
	dir := t.TempDir()
	keysPath := filepath.Join(dir, "keys.json")
	opt := migrate.IAMOptions{
		Source: src, Targets: []migrate.Endpoint{{URL: ds.URL, Region: "us-east-1"}},
		AdminUser: "admin", AdminPassword: "admin",
		KeysFile: keysPath, ReportDir: filepath.Join(dir, "report"),
	}
	rep, err := migrate.RunIAM(t.Context(), opt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Groups) != 5 {
		t.Fatalf("groups %+v", rep.Groups)
	}
	for _, g := range rep.Groups {
		if g.Name == "g4" && (g.Status != "disabled" || g.PolicyApplied) {
			t.Fatalf("disabled group %+v", g)
		}
	}
	var sawOff, sawExp, sawNarrow bool
	for _, sa := range rep.ServiceAccounts {
		switch sa.AccessKey {
		case "saOff":
			sawOff = sa.Inline == "disabled"
		case "saExp":
			sawExp = sa.Expiration != "" && sa.Inline == "implied"
		case "saNar":
			sawNarrow = sa.Inline == "applied" || sa.Inline == "narrowed"
		}
	}
	if !sawOff || !sawExp || !sawNarrow {
		t.Fatalf("service accounts %+v", rep.ServiceAccounts)
	}
	keys := readKeyFile(t, keysPath)
	u03 := keyFor(keys, "u03", "user")
	if u03.SecretKey == "" || u03.Password == "" {
		t.Fatalf("u03 key %+v", u03)
	}
	if keyFor(keys, "uparent", "saOff").SecretKey != "" {
		t.Fatal("disabled service account received a key")
	}
	exp := keyFor(keys, "uparent", "saExp")
	if exp.ExpiresAt == "" {
		t.Fatal("service account expiration was dropped")
	}
	assertStatus := func(row iamKeyRow, method, path string, allow bool) {
		t.Helper()
		got := signedStatus(t, ds.URL, row.AccessKey, row.SecretKey, method, path, nil)
		if allow && got != http.StatusOK {
			t.Fatalf("%s %s %s: got %d, want 200", row.Username, method, path, got)
		}
		if !allow && got < 300 {
			t.Fatalf("%s %s %s: got %d, want deny", row.Username, method, path, got)
		}
	}
	u21 := keyFor(keys, "u21", "user")
	u05 := keyFor(keys, "u05", "user")
	uden := keyFor(keys, "uden", "user")
	uip := keyFor(keys, "uip", "user")
	uparent := keyFor(keys, "uparent", "user")
	unolist := keyFor(keys, "unolist", "user")
	saImp := keyFor(keys, "uparent", "saImp")
	saNar := keyFor(keys, "uparent", "saNar")
	for _, row := range []iamKeyRow{u03, u21} {
		assertStatus(row, http.MethodGet, "/bkt1", true)
		assertStatus(row, http.MethodGet, "/bkt1/pub/a", false)
		assertStatus(row, http.MethodGet, "/bkt1/priv/a", false)
		assertStatus(row, http.MethodGet, "/", false)
	}
	assertStatus(u05, http.MethodGet, "/bkt1/pub/a", true)
	assertStatus(u05, http.MethodGet, "/bkt1/home/a", true)
	assertStatus(u05, http.MethodGet, "/bkt1/priv/a", false)
	assertStatus(u05, http.MethodGet, "/", false)
	assertStatus(uden, http.MethodGet, "/bkt1/pub/a", false)
	assertStatus(uip, http.MethodGet, "/bkt1/pub/a", false)
	assertStatus(uip, http.MethodGet, "/bkt1", false)
	assertStatus(uparent, http.MethodGet, "/bkt1/priv/a", true)
	assertStatus(uparent, http.MethodGet, "/", false)
	assertStatus(unolist, http.MethodGet, "/bkt1/pub/a", true)
	assertStatus(unolist, http.MethodGet, "/bkt1/priv/a", false)
	assertStatus(unolist, http.MethodGet, "/", false)
	assertStatus(saImp, http.MethodGet, "/bkt1/priv/a", true)
	assertStatus(saNar, http.MethodGet, "/bkt1", true)
	assertStatus(saNar, http.MethodGet, "/bkt1/priv/a", false)
	if signedStatus(t, ds.URL, keeperKey, keeperSecret, http.MethodPut, "/bkt1/keeper.txt", strings.NewReader("x")) != http.StatusOK {
		t.Fatal("existing write grant was not kept")
	}
	grants := getGrants(t, ds.URL, admin, "bkt1")
	if !strings.Contains(grants, keeperID) || !strings.Contains(grants, `"can_write":true`) {
		t.Fatalf("grants %s", grants)
	}
	pol := getIAMPolicy(t, ds.URL, admin, "bkt1")
	var box struct {
		Policy string `json:"policy"`
	}
	if err := json.Unmarshal([]byte(pol), &box); err != nil {
		t.Fatal(err)
	}
	merged, err := migrate.MergeBucketPolicy(box.Policy, []policy.Statement{{
		Sid: "keep-me", Effect: "Allow", Action: []string{"s3:GetObject"}, Resource: []string{"arn:aws:s3:::bkt1/keep-me/*"},
		Principal: json.RawMessage(`{"AWS":["arn:aws:iam::datasafe:user/keeper"]}`),
	}})
	if err != nil {
		t.Fatal(err)
	}
	putPolicy(t, ds.URL, admin, "bkt1", merged)
	rep, err = migrate.RunIAM(t.Context(), opt)
	if err != nil {
		t.Fatalf("rerun: %v warnings %v errors %v", err, rep.Warnings, rep.Errors)
	}
	again := readKeyFile(t, keysPath)
	if keyFor(again, "u03", "user").SecretKey != u03.SecretKey || keyFor(again, "u03", "user").Password != u03.Password {
		t.Fatal("rerun replaced the keys file")
	}
	pol = getIAMPolicy(t, ds.URL, admin, "bkt1")
	if !strings.Contains(pol, "keep-me") {
		t.Fatalf("policy shrank: %s", pol)
	}
	raw, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatal(err)
	}
	var stripped []iamKeyRow
	for _, row := range again {
		if row.Username == "u21" {
			continue
		}
		stripped = append(stripped, row)
	}
	out, _ := json.MarshalIndent(stripped, "", "  ")
	if err := os.WriteFile(keysPath, append(out, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	del, _ := http.NewRequest(http.MethodDelete, ds.URL+"/api/v1/keys/"+u21.AccessKey, nil)
	del.Header.Set("Authorization", "Bearer "+admin)
	dresp, err := http.DefaultClient.Do(del)
	if err != nil {
		t.Fatal(err)
	}
	dresp.Body.Close()
	rep, err = migrate.RunIAM(t.Context(), opt)
	if err != nil {
		t.Fatalf("interrupted rerun: %v %+v", err, rep.Errors)
	}
	restored := readKeyFile(t, keysPath)
	if keyFor(restored, "u03", "user").SecretKey != u03.SecretKey {
		t.Fatal("interrupted rerun lost the first key")
	}
	u21b := keyFor(restored, "u21", "user")
	if u21b.SecretKey == "" || u21b.AccessKey == u21.AccessKey {
		t.Fatalf("missing key was not issued: %+v", u21b)
	}
	assertStatus(u21b, http.MethodGet, "/bkt1", true)
	assertStatus(u21b, http.MethodGet, "/bkt1/priv/a", false)
	if strings.Contains(string(raw), `"password":null`) {
		t.Fatal("keys file stored a null password")
	}
}

type iamKeyRow struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Source    string `json:"source"`
	ExpiresAt string `json:"expires_at"`
}

func readKeyFile(t *testing.T, path string) []iamKeyRow {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []iamKeyRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func keyFor(rows []iamKeyRow, user, source string) iamKeyRow {
	for _, row := range rows {
		src := row.Source
		if src == "" {
			src = "user"
		}
		if row.Username == user && src == source {
			return row
		}
	}
	return iamKeyRow{}
}

func signedStatus(t *testing.T, base, access, secret, method, path string, body io.Reader) int {
	t.Helper()
	return signedQuery(t, base, access, secret, method, path, nil, body)
}

func putObject(t *testing.T, base, access, secret, path, body string) {
	t.Helper()
	if signedStatus(t, base, access, secret, http.MethodPut, "/"+path, strings.NewReader(body)) != http.StatusOK {
		t.Fatalf("put %s", path)
	}
}

func createIAMUser(t *testing.T, base, token, name string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/users", strings.NewReader(`{"username":"`+name+`","password":"keepersecretkeeper","role":"user","status":"active"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var box struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&box); err != nil || box.ID == "" {
		t.Fatalf("create user %v", err)
	}
	return box.ID
}

func importIAMKey(t *testing.T, base, token, userID, access, secret string) (string, string) {
	t.Helper()
	raw := fmt.Sprintf(`{"access_key":%q,"secret_key":%q}`, access, secret)
	req, _ := http.NewRequest(http.MethodPost, base+"/api/v1/users/"+userID+"/access-keys", strings.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("import key %d %s", resp.StatusCode, b)
	}
	return access, secret
}

func putGrant(t *testing.T, base, token, bucket, userID string, write bool) {
	t.Helper()
	raw := fmt.Sprintf(`{"grants":[{"user_id":%q,"can_read":true,"can_write":%t}]}`, userID, write)
	req, _ := http.NewRequest(http.MethodPut, base+"/api/v1/buckets/"+bucket+"/access", strings.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("grant %d %s", resp.StatusCode, b)
	}
}

func getGrants(t *testing.T, base, token, bucket string) string {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, base+"/api/v1/buckets/"+bucket+"/access", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("grants %d %s", resp.StatusCode, b)
	}
	return string(b)
}

func putPolicy(t *testing.T, base, token, bucket, document string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"policy": document})
	req, _ := http.NewRequest(http.MethodPut, base+"/api/v1/buckets/"+bucket+"/policy", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put policy %d %s", resp.StatusCode, b)
	}
}

func startRightsMinIO(t *testing.T) migrate.Endpoint {
	t.Helper()
	exp := time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339)
	users := map[string]any{
		"u03":     map[string]any{"status": "enabled", "policyName": "plist", "memberOf": []string{"g4"}},
		"u21":     map[string]any{"status": "enabled", "policyName": "plist"},
		"u05":     map[string]any{"status": "enabled", "policyName": "pprefix"},
		"uden":    map[string]any{"status": "enabled", "policyName": "pdeny"},
		"uip":     map[string]any{"status": "enabled", "policyName": "pip"},
		"uparent": map[string]any{"status": "enabled", "policyName": "pparent"},
		"unolist": map[string]any{"status": "enabled", "policyName": "pobj"},
	}
	policies := map[string]string{
		"plist":   `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1"]}]}`,
		"pprefix": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":"arn:aws:s3:::bkt1"},{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bkt1/pub/*","arn:aws:s3:::bkt1/home/*"]}]}`,
		"pdeny":   `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject","s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1","arn:aws:s3:::bkt1/*"]},{"Sid":"time","Effect":"Deny","Action":"s3:GetObject","Resource":"arn:aws:s3:::bkt1/*","Condition":{"DateGreaterThan":{"aws:CurrentTime":"2000-01-01T00:00:00Z"}}}]}`,
		"pip":     `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject","s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1","arn:aws:s3:::bkt1/*"],"Condition":{"IpAddress":{"aws:SourceIp":"203.0.113.9"}}}]}`,
		"pparent": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject"],"Resource":["arn:aws:s3:::bkt1","arn:aws:s3:::bkt1/*"]}]}`,
		"pobj":    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":"arn:aws:s3:::bkt1/pub/*"}]}`,
		"ppriv":   `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":"arn:aws:s3:::bkt1/priv/*"}]}`,
		"pnar":    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":"arn:aws:s3:::bkt1"}]}`,
	}
	groups := []string{"g0", "g1", "g2", "g3", "g4"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		switch {
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
			status := "enabled"
			var members []string
			policyName := ""
			if g == "g4" {
				status = "disabled"
				members = []string{"u03"}
				policyName = "ppriv"
			}
			body, _ = json.Marshal(map[string]any{"name": g, "status": status, "members": members, "policy": policyName})
		case strings.HasSuffix(r.URL.Path, "/list-canned-policies"):
			raw := map[string]json.RawMessage{}
			for name, doc := range policies {
				raw[name] = json.RawMessage(doc)
			}
			body, _ = json.Marshal(raw)
		case strings.HasSuffix(r.URL.Path, "/list-service-accounts"):
			if r.URL.Query().Get("user") == "uparent" {
				body = []byte(`{"accounts":[{"accessKey":"saImp","expiration":"0001-01-01T00:00:00Z"},{"accessKey":"saNar","expiration":"1970-01-01T00:00:00Z"},{"accessKey":"saOff"},{"accessKey":"saExp","expiration":"` + exp + `"}]}`)
			} else {
				body = []byte(`{"accounts":[]}`)
			}
		case strings.HasSuffix(r.URL.Path, "/info-service-account"):
			switch r.URL.Query().Get("accessKey") {
			case "saNar":
				body = []byte(`{"parentUser":"uparent","accountStatus":"enabled","impliedPolicy":false,"policy":` + jsonQuote(policies["pnar"]) + `}`)
			case "saOff":
				body = []byte(`{"parentUser":"uparent","accountStatus":"disabled","impliedPolicy":true}`)
			case "saExp":
				body = []byte(`{"parentUser":"uparent","accountStatus":"enabled","impliedPolicy":true,"expiration":"` + exp + `"}`)
			default:
				body = []byte(`{"parentUser":"uparent","accountStatus":"enabled","impliedPolicy":true}`)
			}
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

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
