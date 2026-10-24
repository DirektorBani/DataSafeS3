package api

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/api/s3"
	"github.com/DirektorBani/datasafe/internal/metadata"
)

func TestHandleObjectReplicaWritesStoredBytes(t *testing.T) {
	holder := newReplicaLab(t, "holder")
	body := "stored-bytes"
	notePlain(t, holder, "box", "a.txt", "", body)

	pub := authedReplica(t, http.MethodPut, "replica-secret", "holder", "box", "a.txt", "", body)
	rr := httptest.NewRecorder()
	holder.Handler().ServeHTTP(rr, pub)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("public listener %d %s", rr.Code, rr.Body.String())
	}

	req := authedReplica(t, http.MethodPut, "replica-secret", "holder", "box", "a.txt", "", body)
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("replica %d %s", rr.Code, rr.Body.String())
	}
	rc, _, err := holder.backend.GetObject(context.Background(), "box", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Fatalf("stored %q", got)
	}

	again := cloneReplica(req, body)
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, again)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("replay %d", rr.Code)
	}

	bad := authedReplica(t, http.MethodPut, "other", "holder", "box", "a.txt", "", body)
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, bad)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("bad mac %d", rr.Code)
	}
}

func TestReplicaRejectsReplayedReadAndTamperedBody(t *testing.T) {
	holder := newReplicaLab(t, "holder")
	body := "stored-bytes"
	notePlain(t, holder, "box", "a.txt", "", body)
	put := authedReplica(t, http.MethodPut, "replica-secret", "holder", "box", "a.txt", "", body)
	rr := httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, put)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("put %d %s", rr.Code, rr.Body.String())
	}

	get := authedReplica(t, http.MethodGet, "replica-secret", "holder", "box", "a.txt", "", "")
	asPut := cloneReplica(get, "TAMPERED")
	asPut.Method = http.MethodPut
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, asPut)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("get replayed as put %d %s", rr.Code, rr.Body.String())
	}
	rc, _, err := holder.backend.GetObject(context.Background(), "box", "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Fatalf("file changed to %q", got)
	}

	tampered := authedReplica(t, http.MethodPut, "replica-secret", "holder", "box", "a.txt", "", body)
	tampered.Body = io.NopCloser(strings.NewReader("TAMPERED"))
	tampered.ContentLength = int64(len("TAMPERED"))
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, tampered)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("tampered body with the old code %d", rr.Code)
	}

	signedBad := authedReplica(t, http.MethodPut, "replica-secret", "holder", "box", "a.txt", "", "TAMPERED")
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, signedBad)
	if rr.Code != http.StatusConflict {
		t.Fatalf("bytes that do not match metadata %d", rr.Code)
	}

	wrong := authedReplica(t, http.MethodPut, "replica-secret", "other-node", "box", "a.txt", "", body)
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, wrong)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("wrong node %d", rr.Code)
	}

	expired := authedReplica(t, http.MethodGet, "replica-secret", "holder", "box", "a.txt", "", "")
	past := strconv.FormatInt(time.Now().Add(-time.Hour).Unix(), 10)
	sum := sha256.Sum256(nil)
	hash := hex.EncodeToString(sum[:])
	nonce := expired.Header.Get("X-DataSafe-Replica-Nonce")
	canon := replicaCanon(http.MethodGet, hash, past, nonce, "holder", "box", "a.txt", "")
	expired.Header.Set("X-DataSafe-Replica-Expires", past)
	expired.Header.Set("X-DataSafe-Replica", replicaSign("replica-secret", canon))
	rr = httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, expired)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expired %d", rr.Code)
	}
}

func TestGetObjectPullsMissingBytesFromPeer(t *testing.T) {
	holder := newReplicaLab(t, "holder")
	empty := newReplicaLab(t, "empty")
	reader := newReplicaLab(t, "reader")
	body := "stored-bytes"
	notePlain(t, holder, "box", "a.txt", "", body)
	notePlain(t, holder, "box", "b.txt", "v1", "versioned-bytes")
	putReplica(t, holder, "box", "a.txt", "", body)
	putReplica(t, holder, "box", "b.txt", "v1", "versioned-bytes")

	req := authedReplica(t, http.MethodGet, "replica-secret", "holder", "box", "a.txt", "", "")
	rr := httptest.NewRecorder()
	holder.InternalHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != body {
		t.Fatalf("get %d %q", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("Content-Length") != strconv.Itoa(len(body)) {
		t.Fatalf("content-length %s", rr.Header().Get("Content-Length"))
	}

	holderSrv := httptest.NewServer(holder.InternalHandler())
	t.Cleanup(holderSrv.Close)
	emptySrv := httptest.NewServer(empty.InternalHandler())
	t.Cleanup(emptySrv.Close)

	notePlain(t, reader, "box", "a.txt", "", body)
	notePlain(t, reader, "box", "b.txt", "v1", "versioned-bytes")
	reader.svc.FetchStoredObject = func(ctx context.Context, bucket, key, versionID string) error {
		return reader.pullFromPeers(ctx, []replicaPeer{
			{ID: "empty", Addr: emptySrv.URL},
			{ID: "holder", Addr: holderSrv.URL},
		}, bucket, key, versionID)
	}
	rc, _, err := reader.svc.GetObject(context.Background(), "box", "a.txt", "")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ := io.ReadAll(rc)
	if string(got) != body {
		t.Fatalf("pulled %q", got)
	}
	if err := reader.pullObjectReplica(context.Background(), replicaPeer{ID: "holder", Addr: holderSrv.URL}, "box", "b.txt", "v1"); err != nil {
		t.Fatal(err)
	}
	rc, _, err = reader.backend.GetObjectVersion(context.Background(), "box", "b.txt", "v1")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	got, _ = io.ReadAll(rc)
	if string(got) != "versioned-bytes" {
		t.Fatalf("version %q", got)
	}
}

func TestGetObjectUnavailableWhenPeersDown(t *testing.T) {
	reader := newReplicaLab(t, "reader")
	notePlain(t, reader, "box", "a.txt", "", "stored-bytes")
	reader.svc.FetchStoredObject = func(context.Context, string, string, string) error {
		return errors.New("all peers down")
	}
	_, _, err := reader.svc.GetObject(context.Background(), "box", "a.txt", "")
	if !errors.Is(err, s3.ErrObjectUnavailable) {
		t.Fatalf("err %v", err)
	}
}

func TestReplicaPeerPortRewrites(t *testing.T) {
	t.Setenv("STORAGE_INTERNAL_PORT", "9002")
	if got := replicaPeerAddr("http://storage-2:9000"); got != "http://storage-2:9002" {
		t.Fatalf("addr %s", got)
	}
}

func TestInternalAddrDefaultsWhenHAEnabled(t *testing.T) {
	t.Setenv("STORAGE_INTERNAL_PORT", "")
	t.Setenv("STORAGE_INTERNAL_ADDR", "")
	t.Setenv("STORAGE_HA_ENABLED", "")
	if got := InternalAddr(); got != "" {
		t.Fatalf("HA off and no value: want no peer listener, got %q", got)
	}
	t.Setenv("STORAGE_HA_ENABLED", "true")
	if got := InternalAddr(); got != DefaultInternalAddr {
		t.Fatalf("HA on: want %q, got %q", DefaultInternalAddr, got)
	}
	if got := replicaPeerAddr("http://storage-2:9000"); got != "http://storage-2:9002" {
		t.Fatalf("peer address must use the derived port, got %s", got)
	}
	t.Setenv("STORAGE_INTERNAL_ADDR", ":9101")
	if got := InternalAddr(); got != ":9101" {
		t.Fatalf("explicit value wins, got %q", got)
	}
}

func TestIsHAObjectRead(t *testing.T) {
	if isHAObjectRead(httptest.NewRequest(http.MethodGet, "/box", nil)) {
		t.Fatal("bucket list is not an object read")
	}
	if !isHAObjectRead(httptest.NewRequest(http.MethodGet, "/box/a.txt", nil)) {
		t.Fatal("object get")
	}
	if !isHAObjectRead(httptest.NewRequest(http.MethodHead, "/box/a.txt", nil)) {
		t.Fatal("object head")
	}
	if isHAObjectRead(httptest.NewRequest(http.MethodPut, "/box/a.txt", nil)) {
		t.Fatal("put is a write")
	}
}

func newReplicaLab(t *testing.T, node string) *Server {
	t.Helper()
	t.Setenv("STORAGE_HA_ENABLED", "false")
	t.Setenv("STORAGE_AUTO_HOME_BUCKET", "false")
	s, err := NewServer(Config{
		DataDir: t.TempDir(), AdminUser: "admin", AdminPassword: "admin",
		JWTSecret: "replica-secret", NodeID: node,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func notePlain(t *testing.T, s *Server, bucket, key, version, body string) {
	t.Helper()
	if _, err := s.meta.GetBucket(bucket); err != nil {
		if err := s.meta.PutBucket(metadata.BucketRecord{Name: bucket, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	sum := md5.Sum([]byte(body))
	rec := metadata.ObjectRecord{
		Bucket: bucket, Key: key, VersionID: version,
		Size: int64(len(body)), ETag: `"` + hex.EncodeToString(sum[:]) + `"`,
		Encryption:   &metadata.ObjectEncryption{Plaintext: true},
		LastModified: time.Now().UTC(),
	}
	var err error
	if version != "" {
		err = s.meta.PutObjectVersioned(rec)
	} else {
		err = s.meta.PutObject(rec)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func putReplica(t *testing.T, s *Server, bucket, key, version, body string) {
	t.Helper()
	req := authedReplica(t, http.MethodPut, s.cfg.JWTSecret, s.clusterNodeID(), bucket, key, version, body)
	rr := httptest.NewRecorder()
	s.InternalHandler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("put %s %d %s", key, rr.Code, rr.Body.String())
	}
}

func authedReplica(t *testing.T, method, secret, node, bucket, key, version, body string) *http.Request {
	t.Helper()
	raw := []byte(body)
	var rdr io.Reader
	if method == http.MethodPut {
		rdr = strings.NewReader(body)
	} else {
		raw = nil
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	exp := strconv.FormatInt(time.Now().Add(20*time.Second).Unix(), 10)
	var nb [8]byte
	_, _ = rand.Read(nb[:])
	nonce := hex.EncodeToString(nb[:])
	u := replicaPath + "?bucket=" + url.QueryEscape(bucket) + "&key=" + url.QueryEscape(key)
	if version != "" {
		u += "&version=" + url.QueryEscape(version)
	}
	req := httptest.NewRequest(method, u, rdr)
	canon := replicaCanon(method, hash, exp, nonce, node, bucket, key, version)
	req.Header.Set("X-DataSafe-Replica", replicaSign(secret, canon))
	req.Header.Set("X-DataSafe-Replica-Expires", exp)
	req.Header.Set("X-DataSafe-Replica-Nonce", nonce)
	req.Header.Set("X-DataSafe-Replica-Node", node)
	req.Header.Set("X-DataSafe-Replica-SHA256", hash)
	return req
}

func cloneReplica(req *http.Request, body string) *http.Request {
	out := req.Clone(req.Context())
	out.Body = io.NopCloser(strings.NewReader(body))
	out.ContentLength = int64(len(body))
	out.Header = req.Header.Clone()
	return out
}
