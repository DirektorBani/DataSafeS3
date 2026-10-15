package migrate_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/migrate"
)

// TestFromMinIOLiveIAMMatrix diffs a signed allow/deny matrix between pgsty/silo
// MinIO and DataSafe. A DataSafe 2xx where MinIO is not 2xx fails the test.
func TestFromMinIOLiveIAMMatrix(t *testing.T) {
	ep := os.Getenv("MINIO_ENDPOINT")
	key := os.Getenv("MINIO_ACCESS_KEY")
	secret := os.Getenv("MINIO_SECRET_KEY")
	if ep == "" || key == "" || secret == "" {
		t.Skip("MINIO_ENDPOINT is not set")
	}
	src := migrate.Endpoint{URL: ep, AccessKey: key, SecretKey: secret, Region: "us-east-1"}
	if err := provisionMinIOIAM(t, src); err != nil {
		t.Fatalf("provision minio: %v", err)
	}
	if signedStatus(t, ep, key, secret, http.MethodPut, "/bkt1", nil) >= 300 {
		t.Fatal("create minio bucket")
	}
	for _, object := range []string{"pub/a", "priv/a", "home/a"} {
		putObject(t, ep, key, secret, "bkt1/"+object, object)
	}

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
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	ds := srv.URL
	admin := loginIAM(t, ds, "admin", "admin")
	req, _ := http.NewRequest(http.MethodPost, ds+"/api/v1/buckets/bkt1", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer "+admin)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	for _, object := range []string{"pub/a", "priv/a", "home/a"} {
		putObject(t, ds, "datasafe", "datasafesecret", "bkt1/"+object, object)
	}
	dir := t.TempDir()
	rep, err := migrate.RunIAM(t.Context(), migrate.IAMOptions{
		Source: src, Targets: []migrate.Endpoint{{URL: ds, Region: "us-east-1"}},
		AdminUser: "admin", AdminPassword: "admin",
		KeysFile: filepath.Join(dir, "keys.json"), ReportDir: filepath.Join(dir, "report"),
	})
	if err != nil {
		t.Fatalf("import: %v errors %v", err, rep.Errors)
	}
	if len(rep.Groups) < 5 {
		t.Fatalf("groups %+v warnings %v", rep.Groups, rep.Warnings)
	}
	keys := readKeyFile(t, filepath.Join(dir, "keys.json"))
	type principal struct {
		name           string
		minioKey       string
		minioSecret    string
		datasafeKey    string
		datasafeSecret string
	}
	secretByUser := map[string]string{
		"u03": "u03secretvalue1", "u21": "u21secretvalue1", "u05": "u05secretvalue1",
		"uden": "udensecretvalue", "uip": "uipsecretvalue1", "uparent": "uparentsecret11",
		"unolist": "unolistsecret11", "unobind": "unobindsecret111",
	}
	var people []principal
	for user, sec := range secretByUser {
		row := keyFor(keys, user, "user")
		if row.SecretKey == "" {
			t.Fatalf("missing datasafe key for %s", user)
		}
		people = append(people, principal{name: user, minioKey: user, minioSecret: sec, datasafeKey: row.AccessKey, datasafeSecret: row.SecretKey})
	}
	for _, sa := range []struct{ name, secret string }{
		{"saImplied", "saImpliedsecret1"},
		{"saNarrow1", "saNarrow1secret1"},
	} {
		row := keyFor(keys, "uparent", sa.name)
		if row.SecretKey == "" {
			t.Fatalf("missing sa %s keys %v", sa.name, keys)
		}
		people = append(people, principal{name: sa.name, minioKey: sa.name, minioSecret: sa.secret, datasafeKey: row.AccessKey, datasafeSecret: row.SecretKey})
	}
	if keyFor(keys, "uparent", "saDisabled").SecretKey != "" {
		t.Fatal("disabled service account was imported as a working key")
	}
	ops := []struct {
		method string
		path   string
		query  url.Values
	}{
		{http.MethodGet, "/", nil},
		{http.MethodGet, "/bkt1", url.Values{"list-type": {"2"}}},
		{http.MethodGet, "/bkt1", url.Values{"list-type": {"2"}, "prefix": {"pub/"}}},
		{http.MethodGet, "/bkt1", url.Values{"list-type": {"2"}, "prefix": {"priv/"}}},
		{http.MethodGet, "/bkt1", url.Values{"list-type": {"2"}, "prefix": {"home/"}}},
		{http.MethodGet, "/bkt1/pub/a", nil},
		{http.MethodGet, "/bkt1/priv/a", nil},
		{http.MethodGet, "/bkt1/home/a", nil},
		{http.MethodHead, "/bkt1/pub/a", nil},
		{http.MethodHead, "/bkt1/priv/a", nil},
		{http.MethodHead, "/bkt1/home/a", nil},
		{http.MethodGet, "/bkt1/pub/missing", nil},
		{http.MethodHead, "/bkt1/pub/missing", nil},
		{http.MethodGet, "/bkt1/priv/missing", nil},
		{http.MethodHead, "/bkt1/home/missing", nil},
	}
	broader := 0
	for _, p := range people {
		for _, op := range ops {
			minioCode := signedQuery(t, ep, p.minioKey, p.minioSecret, op.method, op.path, op.query, nil)
			dsCode := signedQuery(t, ds, p.datasafeKey, p.datasafeSecret, op.method, op.path, op.query, nil)
			// An imported user with no bucket policy is not bound. GET / keeps
			// the v1.5.0 listing, which is 200 where MinIO is 403.
			if p.name == "unobind" && op.method == http.MethodGet && op.path == "/" && len(op.query) == 0 {
				if dsCode < 200 || dsCode >= 300 {
					t.Errorf("unbound key list %s %d", p.name, dsCode)
				}
				continue
			}
			if dsCode >= 200 && dsCode < 300 && (minioCode < 200 || minioCode >= 300) {
				broader++
				label := op.path
				if len(op.query) > 0 {
					label += "?" + op.query.Encode()
				}
				t.Errorf("broader %s %s %s minio %d datasafe %d", p.name, op.method, label, minioCode, dsCode)
			}
		}
	}
	if broader > 0 {
		t.Fatalf("%d broader cells", broader)
	}
}

func signedQuery(t *testing.T, base, access, secret, method, path string, q url.Values, body io.Reader) int {
	t.Helper()
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		t.Fatal(err)
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(t.Context(), method, u.String(), body)
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SignRequest(req, auth.Credentials{AccessKey: access, SecretKey: secret}, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func provisionMinIOIAM(t *testing.T, src migrate.Endpoint) error {
	t.Helper()
	policies := map[string]string{
		"plist":   `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1"]}]}`,
		"pprefix": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1"]},{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bkt1/pub/*","arn:aws:s3:::bkt1/home/*"]}]}`,
		"pdeny":   `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject","s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1","arn:aws:s3:::bkt1/*"]},{"Sid":"time","Effect":"Deny","Action":"s3:GetObject","Resource":"arn:aws:s3:::bkt1/*","Condition":{"DateGreaterThan":{"aws:CurrentTime":"2000-01-01T00:00:00Z"}}}]}`,
		"pip":     `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject","s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1","arn:aws:s3:::bkt1/*"],"Condition":{"IpAddress":{"aws:SourceIp":"203.0.113.9/32"}}}]}`,
		"pparent": `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject"],"Resource":["arn:aws:s3:::bkt1","arn:aws:s3:::bkt1/*"]}]}`,
		"pobj":    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bkt1/pub/*"]}]}`,
		"ppriv":   `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:GetObject"],"Resource":["arn:aws:s3:::bkt1/priv/*"]}]}`,
		"pnar":    `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Action":["s3:ListBucket"],"Resource":["arn:aws:s3:::bkt1"]}]}`,
	}
	for name, doc := range policies {
		if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/add-canned-policy", url.Values{"name": {name}}, []byte(doc), false); err != nil {
			return err
		}
	}
	users := []struct{ name, secret, policy string }{
		{"u03", "u03secretvalue1", "plist"},
		{"u21", "u21secretvalue1", "plist"},
		{"u05", "u05secretvalue1", "pprefix"},
		{"uden", "udensecretvalue", "pdeny"},
		{"uip", "uipsecretvalue1", "pip"},
		{"uparent", "uparentsecret11", "pparent"},
		{"unolist", "unolistsecret11", "pobj"},
		{"unobind", "unobindsecret111", ""},
	}
	for _, u := range users {
		body, _ := json.Marshal(map[string]string{"secretKey": u.secret, "status": "enabled"})
		if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/add-user", url.Values{"accessKey": {u.name}}, body, true); err != nil {
			return err
		}
		if u.policy == "" {
			continue
		}
		if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/set-user-or-group-policy", url.Values{
			"policyName": {u.policy}, "userOrGroup": {u.name}, "isGroup": {"false"},
		}, nil, false); err != nil {
			return err
		}
	}
	for i := 0; i < 5; i++ {
		name := "g" + string(rune('0'+i))
		members := []string{}
		if name == "g4" {
			members = []string{"u03"}
		}
		body, _ := json.Marshal(map[string]any{"group": name, "members": members, "groupStatus": "enabled", "isRemove": false})
		if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/update-group-members", nil, body, false); err != nil {
			return err
		}
	}
	if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/set-user-or-group-policy", url.Values{
		"policyName": {"ppriv"}, "userOrGroup": {"g4"}, "isGroup": {"true"},
	}, nil, false); err != nil {
		return err
	}
	if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/set-group-status", url.Values{"group": {"g4"}, "status": {"disabled"}}, nil, false); err != nil {
		return err
	}
	exp := time.Now().UTC().Add(2 * time.Hour)
	sas := []struct {
		key, secret, policy string
		expires             *time.Time
	}{
		{key: "saImplied", secret: "saImpliedsecret1"},
		{key: "saNarrow1", secret: "saNarrow1secret1", policy: policies["pnar"]},
		{key: "saDisabled", secret: "saDisabledsecret"},
		{key: "saExpires", secret: "saExpiressecret1", expires: &exp},
	}
	for _, sa := range sas {
		req := map[string]any{"targetUser": "uparent", "accessKey": sa.key, "secretKey": sa.secret}
		if sa.policy != "" {
			req["policy"] = json.RawMessage(sa.policy)
		}
		if sa.expires != nil {
			req["expiration"] = sa.expires.UTC().Format(time.RFC3339)
		}
		body, _ := json.Marshal(req)
		if err := minioAdmin(t, src, http.MethodPut, "/minio/admin/v3/add-service-account", nil, body, true); err != nil {
			return err
		}
	}
	off, _ := json.Marshal(map[string]string{"newStatus": "disabled"})
	return minioAdmin(t, src, http.MethodPost, "/minio/admin/v3/update-service-account", url.Values{"accessKey": {"saDisabled"}}, off, true)
}

func minioAdmin(t *testing.T, src migrate.Endpoint, method, path string, q url.Values, body []byte, encrypt bool) error {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		if encrypt {
			sealed, err := migrate.EncryptAdmin(src.SecretKey, body)
			if err != nil {
				return err
			}
			rdr = bytes.NewReader(sealed)
		} else {
			rdr = bytes.NewReader(body)
		}
	}
	u, err := url.Parse(strings.TrimRight(src.URL, "/") + path)
	if err != nil {
		return err
	}
	if len(q) > 0 {
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(t.Context(), method, u.String(), rdr)
	if err != nil {
		return err
	}
	if err := auth.SignRequest(req, auth.Credentials{AccessKey: src.AccessKey, SecretKey: src.SecretKey}, "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return errStatus(method, path, resp.StatusCode, raw)
	}
	return nil
}

type statusErr struct {
	method, path string
	code         int
	body         string
}

func errStatus(method, path string, code int, body []byte) error {
	return &statusErr{method: method, path: path, code: code, body: strings.TrimSpace(string(body))}
}

func (e *statusErr) Error() string {
	return e.method + " " + e.path + " http " + strconv.Itoa(e.code) + " " + e.body
}
