package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/auth"
	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

func rootCreds() auth.Credentials {
	return auth.Credentials{AccessKey: "datasafe", SecretKey: "datasafesecret"}
}

func newKeyedServer(t *testing.T, dir, key string) *api.Server {
	t.Helper()
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NATS_URL", "")
	s, err := api.NewServer(api.Config{
		DataDir:       dir,
		AdminUser:     "admin",
		AdminPassword: "admin",
		JWTSecret:     "test",
		SSEMasterKey:  key,
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
	cfg.AdminFirstLoginCompleted = true
	if err := s.Meta().PutSystemConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return s
}

func s3Call(t *testing.T, s *api.Server, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "localhost"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if err := auth.SignRequest(req, rootCreds(), "us-east-1", "s3", "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func createBucket(t *testing.T, s *api.Server, token, name string, body []byte) {
	t.Helper()
	if body == nil {
		body = []byte(`{}`)
	}
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/"+name, token, body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s %d %s", name, rec.Code, rec.Body.String())
	}
}

func storageKeyOf(t *testing.T, s *api.Server, name string) string {
	t.Helper()
	rec, err := s.Meta().GetBucket(name)
	if err != nil {
		t.Fatal(err)
	}
	return rec.EffectiveStorageKey()
}

func TestE01ConsoleMatchesS3Encryption(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "encbox", nil)

	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/encbox/encryption", tok, []byte(`{"enabled":true}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("console enable %d %s", rec.Code, rec.Body.String())
	}
	var view map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view["algorithm"] != "AES256" || view["enabled"] != true {
		t.Fatalf("console view %#v", view)
	}
	keyID, _ := view["key_id"].(string)
	got := s3Call(t, s, http.MethodGet, "/encbox?encryption", nil, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "AES256") || !strings.Contains(got.Body.String(), keyID) {
		t.Fatalf("s3 get after console %d %s", got.Code, got.Body.String())
	}

	createBucket(t, s, tok, "kmsbox", nil)
	xmlBody := []byte(`<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>aws:kms</SSEAlgorithm><KMSMasterKeyID>datasafe-default-kmsbox</KMSMasterKeyID></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	put := s3Call(t, s, http.MethodPut, "/kmsbox?encryption", xmlBody, nil)
	if put.Code != http.StatusOK {
		t.Fatalf("s3 put kms %d %s", put.Code, put.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/v1/buckets/kmsbox/encryption", tok, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("console get %d %s", rec.Code, rec.Body.String())
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &view)
	if view["algorithm"] != "aws:kms" || view["key_id"] != "datasafe-default-kmsbox" {
		t.Fatalf("console after s3 %#v", view)
	}
}

func TestE01EncryptNewKeepOldAndStepUp(t *testing.T) {
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "e01-master")
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "mixbox", nil)
	sk := storageKeyOf(t, s, "mixbox")

	old := s3Call(t, s, http.MethodPut, "/mixbox/old.txt", []byte("old-plain"), nil)
	if old.Code != http.StatusOK {
		t.Fatalf("put old %d %s", old.Code, old.Body.String())
	}
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mixbox/encryption", tok, []byte(`{"enabled":true}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("enable %d %s", rec.Code, rec.Body.String())
	}
	neu := s3Call(t, s, http.MethodPut, "/mixbox/new.txt", []byte("new-sealed"), nil)
	if neu.Code != http.StatusOK || neu.Header().Get("x-amz-server-side-encryption") != "AES256" {
		t.Fatalf("put new %d sse %q body %s", neu.Code, neu.Header().Get("x-amz-server-side-encryption"), neu.Body.String())
	}
	gotOld := s3Call(t, s, http.MethodGet, "/mixbox/old.txt", nil, nil)
	if gotOld.Code != http.StatusOK || gotOld.Body.String() != "old-plain" || gotOld.Header().Get("x-amz-server-side-encryption") != "" {
		t.Fatalf("old get %d sse %q %q", gotOld.Code, gotOld.Header().Get("x-amz-server-side-encryption"), gotOld.Body.String())
	}
	gotNew := s3Call(t, s, http.MethodGet, "/mixbox/new.txt", nil, nil)
	if gotNew.Code != http.StatusOK || gotNew.Body.String() != "new-sealed" || gotNew.Header().Get("Content-Length") != "10" {
		t.Fatalf("new get %d len %s %q", gotNew.Code, gotNew.Header().Get("Content-Length"), gotNew.Body.String())
	}
	rawOld, err := os.ReadFile(filepath.Join(dir, "objects", "buckets", sk, "objects", "old.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rawOld) != "old-plain" {
		t.Fatalf("old disk changed %q", rawOld)
	}
	rawNew, err := os.ReadFile(filepath.Join(dir, "objects", "buckets", sk, "objects", "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(rawNew, []byte("DSSE")) {
		t.Fatalf("new disk prefix %q", rawNew[:min(8, len(rawNew))])
	}

	putMFAPolicy(t, s, tok, 0, false, false)
	denied := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mixbox/encryption", tok, []byte(`{"enabled":false}`), "")
	if denied.Code != http.StatusForbidden || !strings.Contains(denied.Body.String(), "mfa_required") {
		t.Fatalf("owner without 2FA %d %s", denied.Code, denied.Body.String())
	}
	secret, _ := enrollTOTP(t, s.Handler(), tok)
	noTok := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mixbox/encryption", tok, []byte(`{"enabled":false}`), "")
	if noTok.Code != http.StatusUnauthorized || !strings.Contains(noTok.Body.String(), "encryption_disable") {
		t.Fatalf("step-up %d %s", noTok.Code, noTok.Body.String())
	}
	action := stepUp(t, s.Handler(), tok, secret, auth.ActionEncryptionDisable, "mixbox", time.Now().Add(30*time.Second))
	off := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mixbox/encryption", tok, []byte(`{"enabled":false}`), action)
	if off.Code != http.StatusOK || !strings.Contains(off.Body.String(), `"enabled":false`) {
		t.Fatalf("disable %d %s", off.Code, off.Body.String())
	}
	still := s3Call(t, s, http.MethodGet, "/mixbox/new.txt", nil, nil)
	if still.Code != http.StatusOK || still.Body.String() != "new-sealed" {
		t.Fatalf("after disable %d %q", still.Code, still.Body.String())
	}
}

func TestE01PublicKMSAndAnonymous(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "pubbox", []byte(`{"visibility":"public-read"}`))
	xmlBody := []byte(`<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>aws:kms</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
	put := s3Call(t, s, http.MethodPut, "/pubbox?encryption", xmlBody, nil)
	if put.Code != http.StatusConflict || !strings.Contains(put.Body.String(), s3.MsgPublicKMS) {
		t.Fatalf("public+kms %d %s", put.Code, put.Body.String())
	}
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/settings/buckets/pubbox", tok, []byte(`{"visibility":"private"}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("make private %d %s", rec.Code, rec.Body.String())
	}
	obj := s3Call(t, s, http.MethodPut, "/pubbox/secret.txt", []byte("kms-body"), map[string]string{
		"x-amz-server-side-encryption": "aws:kms",
	})
	if obj.Code != http.StatusOK {
		t.Fatalf("put kms object %d %s", obj.Code, obj.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/settings/buckets/pubbox", tok, []byte(`{"visibility":"public-read"}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("public again %d %s", rec.Code, rec.Body.String())
	}
	anon := httptest.NewRequest(http.MethodGet, "/pubbox/secret.txt", nil)
	arec := httptest.NewRecorder()
	s.Handler().ServeHTTP(arec, anon)
	if arec.Code != http.StatusForbidden || !strings.Contains(arec.Body.String(), s3.MsgKMSPublic) {
		t.Fatalf("anon %d %s", arec.Code, arec.Body.String())
	}
	authGet := s3Call(t, s, http.MethodGet, "/pubbox/secret.txt", nil, nil)
	if authGet.Code != http.StatusOK || authGet.Body.String() != "kms-body" {
		t.Fatalf("auth get %d %q", authGet.Code, authGet.Body.String())
	}
}

func TestE01ShareRevokedAndDestroyed(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "sharekms", nil)
	obj := s3Call(t, s, http.MethodPut, "/sharekms/file.txt", []byte("shared-secret"), map[string]string{
		"x-amz-server-side-encryption": "aws:kms",
	})
	if obj.Code != http.StatusOK {
		t.Fatalf("put %d %s", obj.Code, obj.Body.String())
	}
	body, _ := json.Marshal(map[string]any{"key": "file.txt", "expires_in_sec": 3600})
	rec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/sharekms/shares", tok, body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("share %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	dl := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
	drec := httptest.NewRecorder()
	s.Handler().ServeHTTP(drec, dl)
	if drec.Code != http.StatusOK || drec.Body.String() != "shared-secret" {
		t.Fatalf("download %d %q", drec.Code, drec.Body.String())
	}
	user, err := s.Meta().GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	user.Status = metadata.StatusSuspended
	if err := s.Meta().UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	dl = httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+created.Share.Token+"/download", nil)
	drec = httptest.NewRecorder()
	s.Handler().ServeHTTP(drec, dl)
	if drec.Code != http.StatusForbidden {
		t.Fatalf("revoked creator %d %s", drec.Code, drec.Body.String())
	}
	user.Status = metadata.StatusActive
	if err := s.Meta().UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	aes := s3Call(t, s, http.MethodPut, "/sharekms/aes.txt", []byte("aes-secret"), map[string]string{
		"x-amz-server-side-encryption": "AES256",
	})
	if aes.Code != http.StatusOK {
		t.Fatalf("aes put %d %s", aes.Code, aes.Body.String())
	}
	plain := s3Call(t, s, http.MethodPut, "/sharekms/plain.txt", []byte("plain-secret"), nil)
	if plain.Code != http.StatusOK {
		t.Fatalf("plain put %d %s", plain.Code, plain.Body.String())
	}
	aesBody, _ := json.Marshal(map[string]any{"key": "aes.txt", "expires_in_sec": 3600})
	plainBody, _ := json.Marshal(map[string]any{"key": "plain.txt", "expires_in_sec": 3600})
	aesRec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/sharekms/shares", tok, aesBody, "")
	plainRec := doJSON(t, s.Handler(), http.MethodPost, "/api/v1/buckets/sharekms/shares", tok, plainBody, "")
	if aesRec.Code != http.StatusCreated || plainRec.Code != http.StatusCreated {
		t.Fatalf("shares aes %d plain %d", aesRec.Code, plainRec.Code)
	}
	var aesCreated, plainCreated struct {
		Share struct {
			Token string `json:"token"`
		} `json:"share"`
	}
	_ = json.Unmarshal(aesRec.Body.Bytes(), &aesCreated)
	_ = json.Unmarshal(plainRec.Body.Bytes(), &plainCreated)
	user.Status = metadata.StatusSuspended
	if err := s.Meta().UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	aesDL := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+aesCreated.Share.Token+"/download", nil)
	aesDRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(aesDRec, aesDL)
	if aesDRec.Code != http.StatusForbidden {
		t.Fatalf("aes revoked %d %s", aesDRec.Code, aesDRec.Body.String())
	}
	plainDL := httptest.NewRequest(http.MethodGet, "/api/v1/public/share/"+plainCreated.Share.Token+"/download", nil)
	plainDRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(plainDRec, plainDL)
	if plainDRec.Code != http.StatusOK || plainDRec.Body.String() != "plain-secret" {
		t.Fatalf("plain link %d %q", plainDRec.Code, plainDRec.Body.String())
	}
	user.Status = metadata.StatusActive
	if err := s.Meta().UpdateUser(user); err != nil {
		t.Fatal(err)
	}
	sk := storageKeyOf(t, s, "sharekms")
	meta, err := s.Meta().GetObject(sk, "file.txt")
	if err != nil || meta.Encryption == nil {
		t.Fatalf("object meta %v %+v", err, meta.Encryption)
	}
	key, err := s.Meta().GetKMSKey(meta.Encryption.KeyID)
	if err != nil {
		t.Fatal(err)
	}
	key.Status = "destroyed"
	if err := s.Meta().PutKMSKey(key); err != nil {
		t.Fatal(err)
	}
	dead := s3Call(t, s, http.MethodGet, "/sharekms/file.txt", nil, nil)
	if dead.Code != http.StatusForbidden || !strings.Contains(dead.Body.String(), s3.MsgKeyDestroyed) {
		t.Fatalf("destroyed %d %s", dead.Code, dead.Body.String())
	}
}

func TestE01EncryptionRBAC(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	seedUser(t, s, "encreader", "enc-pass-1")
	createBucket(t, s, tok, "rbacbox", nil)
	grants := []byte(`{"grants":[{"user_id":"encreader-id","level":"read","can_read":true}]}`)
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/rbacbox/access", tok, grants, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("grant %d %s", rec.Code, rec.Body.String())
	}
	reader := loginToken(t, s, "encreader", "enc-pass-1")
	rec = doJSON(t, s.Handler(), http.MethodGet, "/api/v1/buckets/rbacbox/encryption", reader, nil, "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"can_edit":false`) {
		t.Fatalf("reader get %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/rbacbox/encryption", reader, []byte(`{"enabled":true}`), "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reader put %d %s", rec.Code, rec.Body.String())
	}
}

func TestE01MultipartAndCopy(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "mpbox", nil)
	createBucket(t, s, tok, "plainbox", nil)
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/mpbox/encryption", tok, []byte(`{"enabled":true}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("enable %d %s", rec.Code, rec.Body.String())
	}
	init := s3Call(t, s, http.MethodPost, "/mpbox/large.bin?uploads", nil, nil)
	if init.Code != http.StatusOK {
		t.Fatalf("init %d %s", init.Code, init.Body.String())
	}
	var initResp struct {
		UploadID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(init.Body.Bytes(), &initResp); err != nil || initResp.UploadID == "" {
		t.Fatalf("upload id %v %s", err, init.Body.String())
	}
	sk := storageKeyOf(t, s, "mpbox")
	mp, err := s.Meta().GetMultipart(initResp.UploadID)
	if err != nil || mp.Encryption == nil || mp.Encryption.WrappedDEK == "" {
		t.Fatalf("multipart dek %v %+v", err, mp.Encryption)
	}
	dek := mp.Encryption.WrappedDEK
	p1 := s3Call(t, s, http.MethodPut, "/mpbox/large.bin?partNumber=1&uploadId="+initResp.UploadID, []byte("aaa"), nil)
	p2 := s3Call(t, s, http.MethodPut, "/mpbox/large.bin?partNumber=2&uploadId="+initResp.UploadID, []byte("bbb"), nil)
	if p1.Code != http.StatusOK || p2.Code != http.StatusOK {
		t.Fatalf("parts %d %d", p1.Code, p2.Code)
	}
	mp, err = s.Meta().GetMultipart(initResp.UploadID)
	if err != nil || mp.Encryption.WrappedDEK != dek {
		t.Fatal("dek changed between parts")
	}
	completeBody := `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>` + p1.Header().Get("ETag") + `</ETag></Part><Part><PartNumber>2</PartNumber><ETag>` + p2.Header().Get("ETag") + `</ETag></Part></CompleteMultipartUpload>`
	done := s3Call(t, s, http.MethodPost, "/mpbox/large.bin?uploadId="+initResp.UploadID, []byte(completeBody), nil)
	if done.Code != http.StatusOK {
		t.Fatalf("complete %d %s", done.Code, done.Body.String())
	}
	got := s3Call(t, s, http.MethodGet, "/mpbox/large.bin", nil, nil)
	if got.Code != http.StatusOK || got.Body.String() != "aaabbb" {
		t.Fatalf("multipart get %d %q", got.Code, got.Body.String())
	}
	obj, err := s.Meta().GetObject(sk, "large.bin")
	if err != nil || obj.Encryption == nil || obj.Encryption.WrappedDEK != dek {
		t.Fatalf("completed dek mismatch %v", err)
	}

	one := s3Call(t, s, http.MethodPut, "/mpbox/one.txt", []byte("copy-me"), nil)
	if one.Code != http.StatusOK {
		t.Fatalf("put one %d %s", one.Code, one.Body.String())
	}
	cp := s3Call(t, s, http.MethodPut, "/plainbox/one.txt", nil, map[string]string{"x-amz-copy-source": "/mpbox/one.txt"})
	if cp.Code != http.StatusOK {
		t.Fatalf("copy %d %s", cp.Code, cp.Body.String())
	}
	dst := s3Call(t, s, http.MethodGet, "/plainbox/one.txt", nil, nil)
	if dst.Code != http.StatusOK || dst.Body.String() != "copy-me" || dst.Header().Get("x-amz-server-side-encryption") != "" {
		t.Fatalf("copy get %d sse %q %q", dst.Code, dst.Header().Get("x-amz-server-side-encryption"), dst.Body.String())
	}
	src := s3Call(t, s, http.MethodGet, "/mpbox/one.txt", nil, nil)
	if src.Body.String() != "copy-me" {
		t.Fatalf("src %q", src.Body.String())
	}
}

func plantUnmarkedObject(t *testing.T, meta metadata.MetadataStore, dir, bucket, key string, body []byte) {
	t.Helper()
	fs, err := storage.NewFSBackend(filepath.Join(dir, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.PutBucket(metadata.BucketRecord{
		Name: bucket, StorageKey: bucket, Owner: "admin", Visibility: "private", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	etag, err := fs.PutObject(context.Background(), bucket, key, bytes.NewReader(body), int64(len(body)), "application/octet-stream")
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.PutObject(metadata.ObjectRecord{
		Bucket: bucket, Key: key, Size: int64(len(body)), ETag: etag,
		ContentType: "application/octet-stream", LastModified: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
}

func assertUpgradePlaintext(t *testing.T, s *api.Server, bucket, key, want string) {
	t.Helper()
	st, err := s.Meta().GetKMSState()
	if err != nil || st.LegacySSEObjects || st.Fingerprint == "" {
		t.Fatalf("state %+v %v", st, err)
	}
	got := s3Call(t, s, http.MethodGet, "/"+bucket+"/"+key, nil, nil)
	if got.Code != http.StatusOK || got.Body.String() != want {
		t.Fatalf("upgrade get %d %q", got.Code, got.Body.String())
	}
}

func TestE01PlaintextUpgradeFrom15Bolt(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "")
	dir := t.TempDir()
	meta, err := metadata.Open(metadata.Config{Backend: metadata.BackendBolt, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	plantUnmarkedObject(t, meta, dir, "upbox", "old.txt", []byte("from-plaintext"))
	_ = meta.Close()

	s := newKeyedServer(t, dir, "upgrade-key")
	assertUpgradePlaintext(t, s, "upbox", "old.txt", "from-plaintext")
}

func TestE01PoisonedLegacyFlagCleared(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "")
	const key = "upgrade-key"
	dir := t.TempDir()
	meta, err := metadata.Open(metadata.Config{Backend: metadata.BackendBolt, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	plantUnmarkedObject(t, meta, dir, "poison", "old.txt", []byte("still-plain"))
	if err := meta.PutKMSState(metadata.KMSState{
		Fingerprint: kms.Fingerprint(key), LegacySSEObjects: true,
	}); err != nil {
		t.Fatal(err)
	}
	_ = meta.Close()

	s := newKeyedServer(t, dir, key)
	assertUpgradePlaintext(t, s, "poison", "old.txt", "still-plain")
}

func TestE01PlaintextUpgradeFrom15Postgres(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "")
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set — skipping postgres integration test")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Skipf("postgres unavailable: %v", err)
	}
	name := fmt.Sprintf("ds_e01_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close()
	})
	fresh, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Path = "/" + name
	dir := t.TempDir()
	meta, err := metadata.Open(metadata.Config{
		Backend: metadata.BackendPostgres, PostgresDSN: fresh.String(), DataDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	plantUnmarkedObject(t, meta, dir, "pgup", "old.txt", []byte("pg-plain"))
	_ = meta.Close()

	start := func() *api.Server {
		t.Helper()
		srv, err := api.NewServer(api.Config{
			DataDir: dir, AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test",
			SSEMasterKey: "upgrade-key",
			Metadata: metadata.Config{
				Backend: metadata.BackendPostgres, PostgresDSN: fresh.String(), DataDir: dir,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return srv
	}
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	t.Setenv("STORAGE_KAFKA_BROKERS", "")
	t.Setenv("STORAGE_NATS_URL", "")
	s := start()
	assertUpgradePlaintext(t, s, "pgup", "old.txt", "pg-plain")
	_ = s.Close()

	meta, err = metadata.Open(metadata.Config{
		Backend: metadata.BackendPostgres, PostgresDSN: fresh.String(), DataDir: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.PutKMSState(metadata.KMSState{
		Fingerprint: kms.Fingerprint("upgrade-key"), LegacySSEObjects: true,
	}); err != nil {
		t.Fatal(err)
	}
	_ = meta.Close()
	s = start()
	t.Cleanup(func() { _ = s.Close() })
	assertUpgradePlaintext(t, s, "pgup", "old.txt", "pg-plain")
}

func TestE01LegacyFormat0AndFingerprint(t *testing.T) {
	t.Setenv("STORAGE_SSE_LEGACY_FORMAT0", "true")
	dir := t.TempDir()
	meta, err := metadata.Open(metadata.Config{Backend: metadata.BackendBolt, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := storage.NewSSECipher("legacy-key")
	if err != nil || cipher == nil {
		t.Fatal(err)
	}
	ct, err := cipher.Encrypt([]byte("from-15"))
	if err != nil {
		t.Fatal(err)
	}
	plantUnmarkedObject(t, meta, dir, "legbox", "old.bin", ct)
	_ = meta.Close()

	s := newKeyedServer(t, dir, "legacy-key")
	st, err := s.Meta().GetKMSState()
	if err != nil || !st.LegacySSEObjects || st.Fingerprint == "" {
		t.Fatalf("legacy state %+v %v", st, err)
	}
	got := s3Call(t, s, http.MethodGet, "/legbox/old.bin", nil, nil)
	if got.Code != http.StatusOK || got.Body.String() != "from-15" {
		t.Fatalf("legacy get %d %q", got.Code, got.Body.String())
	}
	fresh := s3Call(t, s, http.MethodPut, "/legbox/new.txt", []byte("fresh"), nil)
	if fresh.Code != http.StatusOK {
		t.Fatalf("fresh put %d %s", fresh.Code, fresh.Body.String())
	}
	freshGet := s3Call(t, s, http.MethodGet, "/legbox/new.txt", nil, nil)
	if freshGet.Body.String() != "fresh" {
		t.Fatalf("fresh get %q", freshGet.Body.String())
	}
	_ = s.Close()

	if _, err := api.NewServer(api.Config{
		DataDir: dir, AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test", SSEMasterKey: "other-key",
	}); err == nil || !strings.Contains(err.Error(), "SSE master key fingerprint mismatch") {
		t.Fatalf("mismatch %v", err)
	}
}

func TestE01EmptyKeySkipsKMSStateAndRejectsClearedKey(t *testing.T) {
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "")
	st, err := s.Meta().GetKMSState()
	if err != nil || st.Fingerprint != "" || st.Initialized || st.LegacySSEObjects {
		t.Fatalf("idle state %+v %v", st, err)
	}
	if s.Svc().LocalKMSConfigured() {
		t.Fatal("local kms configured without a master key")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.StartBackground(ctx)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		beats, err := s.Meta().ListNodeHeartbeats(time.Now().Add(-time.Minute))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if len(beats) != 0 {
			cancel()
			t.Fatalf("idle heartbeats %+v", beats)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = s.Close()

	meta, err := metadata.Open(metadata.Config{Backend: metadata.BackendBolt, DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := meta.PutKMSState(metadata.KMSState{Fingerprint: kms.Fingerprint("kept")}); err != nil {
		_ = meta.Close()
		t.Fatal(err)
	}
	_ = meta.Close()
	srv, err := api.NewServer(api.Config{
		DataDir: dir, AdminUser: "admin", AdminPassword: "admin", JWTSecret: "test",
	})
	if srv != nil {
		_ = srv.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "SSE master key fingerprint mismatch") {
		t.Fatalf("cleared key %v", err)
	}
}

func TestE01HeartbeatAdvertisesAtLeast160(t *testing.T) {
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "hb-key")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.StartBackground(ctx)
	deadline := time.Now().Add(2 * time.Second)
	var beats []metadata.NodeHeartbeat
	for {
		var err error
		beats, err = s.Meta().ListNodeHeartbeats(time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		if len(beats) > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(beats) != 1 || beats[0].Version != "1.6.0" {
		t.Fatalf("beats %+v", beats)
	}
}

func TestE01KeyAddedLaterStaysPlaintext(t *testing.T) {
	dir := t.TempDir()
	s := newKeyedServer(t, dir, "")
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "later", nil)
	put := s3Call(t, s, http.MethodPut, "/later/a.txt", []byte("still-plain"), nil)
	if put.Code != http.StatusOK {
		t.Fatalf("put %d %s", put.Code, put.Body.String())
	}
	_ = s.Close()
	s = newKeyedServer(t, dir, "added-later")
	st, err := s.Meta().GetKMSState()
	if err != nil || st.LegacySSEObjects || st.Fingerprint == "" {
		t.Fatalf("state %+v %v", st, err)
	}
	got := s3Call(t, s, http.MethodGet, "/later/a.txt", nil, nil)
	if got.Code != http.StatusOK || got.Body.String() != "still-plain" {
		t.Fatalf("get %d %q", got.Code, got.Body.String())
	}
}

func TestE01ClusterUpgradeGate(t *testing.T) {
	s := testServer(t)
	tok := loginToken(t, s, "admin", "admin")
	createBucket(t, s, tok, "upbox", nil)
	if err := s.Meta().UpsertNodeHeartbeat("old-node", "1.5.0", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/upbox/encryption", tok, []byte(`{"enabled":true}`), "")
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), s3.MsgClusterUpgrade) {
		t.Fatalf("upgrade %d %s", rec.Code, rec.Body.String())
	}
	if err := s.Meta().UpsertNodeHeartbeat("old-node", "1.6.0", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rec = doJSON(t, s.Handler(), http.MethodPut, "/api/v1/buckets/upbox/encryption", tok, []byte(`{"enabled":true}`), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("after upgrade %d %s", rec.Code, rec.Body.String())
	}
}
