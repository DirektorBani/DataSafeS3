package api

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/ha"
	"github.com/DirektorBani/datasafe/internal/metadata"
	"github.com/DirektorBani/datasafe/internal/storage"
)

const (
	replicaPath = "/api/v1/ha/object-replica"
	// replicaTTL is the longest a signed peer request stays valid.
	replicaTTL = 30 * time.Second
)

// InternalHandler serves the peer object API. The public listener and the
// load balancer do not. STORAGE_INTERNAL_ADDR is that listener.
func (s *Server) InternalHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != replicaPath {
			http.NotFound(w, r)
			return
		}
		s.handleObjectReplica(w, r)
	})
}

func emptyBodySHA() string {
	sum := sha256.Sum256(nil)
	return hex.EncodeToString(sum[:])
}

// replicaCanon binds a peer request to the verb, the path, the body, the
// expiry, a nonce, and the destination node. A captured GET cannot be
// replayed as a PUT, and a different body does not match the code.
func replicaCanon(method, bodyHash, expiry, nonce, nodeID, bucket, key, version string) string {
	return strings.Join([]string{
		method, replicaPath, bodyHash, expiry, nonce, nodeID, bucket, key, version,
	}, "\n")
}

func replicaSign(secret, canon string) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(canon))
	return hex.EncodeToString(m.Sum(nil))
}

func replicaMACOK(secret, canon, got string) bool {
	if secret == "" || got == "" {
		return false
	}
	want := replicaSign(secret, canon)
	a, errA := hex.DecodeString(want)
	b, errB := hex.DecodeString(got)
	if errA != nil || errB != nil || len(a) != len(b) {
		return false
	}
	return hmac.Equal(a, b)
}

func replicaExpiryOK(expUnix int64, now time.Time) bool {
	exp := time.Unix(expUnix, 0)
	if exp.Before(now.Add(-5 * time.Second)) {
		return false
	}
	if exp.After(now.Add(replicaTTL)) {
		return false
	}
	return true
}

type replicaNonces struct {
	mu   sync.Mutex
	seen map[string]int64
}

func (n *replicaNonces) accept(nonce string, exp int64) bool {
	if n == nil || nonce == "" {
		return false
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.seen == nil {
		n.seen = map[string]int64{}
	}
	now := time.Now().Unix()
	if len(n.seen) > 256 {
		for k, e := range n.seen {
			if e < now {
				delete(n.seen, k)
			}
		}
	}
	if _, ok := n.seen[nonce]; ok {
		return false
	}
	n.seen[nonce] = exp
	return true
}

func (s *Server) replicaNonce() *replicaNonces {
	s.replicaNonceOnce.Do(func() { s.replicaSeen = &replicaNonces{} })
	return s.replicaSeen
}

func (s *Server) signReplica(req *http.Request, nodeID, bucket, key, version, bodyHash string) {
	exp := strconv.FormatInt(time.Now().Add(20*time.Second).Unix(), 10)
	var nb [16]byte
	_, _ = rand.Read(nb[:])
	nonce := hex.EncodeToString(nb[:])
	canon := replicaCanon(req.Method, bodyHash, exp, nonce, nodeID, bucket, key, version)
	req.Header.Set("X-DataSafe-Replica", replicaSign(s.cfg.JWTSecret, canon))
	req.Header.Set("X-DataSafe-Replica-Expires", exp)
	req.Header.Set("X-DataSafe-Replica-Nonce", nonce)
	req.Header.Set("X-DataSafe-Replica-Node", nodeID)
	req.Header.Set("X-DataSafe-Replica-SHA256", bodyHash)
}

func etagEqual(stored, md5hex string) bool {
	stored = strings.Trim(stored, `"`)
	return stored != "" && strings.EqualFold(stored, md5hex)
}

// replicaPayloadOK accepts stored bytes that match the metadata row.
// A sealed object's Size is the plaintext length, so the check is the ETag
// of the bytes on disk. An unmarked object must also match that length.
func replicaPayloadOK(rec metadata.ObjectRecord, n int64, md5hex string) bool {
	if !etagEqual(rec.ETag, md5hex) {
		return false
	}
	if rec.Encryption != nil && rec.Encryption.Sealed() {
		return true
	}
	return rec.Size == n
}

func (s *Server) replicaMeta(bucket, key, version string) (metadata.ObjectRecord, error) {
	if version != "" {
		return s.meta.GetObjectVersion(bucket, key, version)
	}
	return s.meta.GetObject(bucket, key)
}

// handleObjectReplica stores or returns bytes that already match metadata.
// It does not update metadata and it does not go through the leader proxy.
func (s *Server) handleObjectReplica(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	bucket := r.URL.Query().Get("bucket")
	key := r.URL.Query().Get("key")
	versionID := r.URL.Query().Get("version")
	if bucket == "" || key == "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	expUnix, err := strconv.ParseInt(r.Header.Get("X-DataSafe-Replica-Expires"), 10, 64)
	if err != nil || !replicaExpiryOK(expUnix, time.Now()) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	nodeID := r.Header.Get("X-DataSafe-Replica-Node")
	if nodeID == "" || nodeID != s.clusterNodeID() {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var bodyHash, md5hex string
	var n int64
	var tmp *os.File
	if r.Method == http.MethodPut {
		tmp, bodyHash, md5hex, n, err = spoolReplica(r.Body)
		if err != nil {
			slog.Error("object replica spool", "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()
	} else {
		bodyHash = emptyBodySHA()
	}
	nonce := r.Header.Get("X-DataSafe-Replica-Nonce")
	canon := replicaCanon(r.Method, bodyHash, r.Header.Get("X-DataSafe-Replica-Expires"), nonce, nodeID, bucket, key, versionID)
	if !replicaMACOK(s.cfg.JWTSecret, canon, r.Header.Get("X-DataSafe-Replica")) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if !s.replicaNonce().accept(nonce, expUnix) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	rec, err := s.replicaMeta(bucket, key, versionID)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		s.serveObjectReplica(w, r, rec)
		return
	}
	if !replicaPayloadOK(rec, n, md5hex) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	var etag string
	if versionID != "" {
		etag, err = s.backend.PutObjectVersion(r.Context(), bucket, key, versionID, tmp, n, "")
	} else {
		etag, err = s.backend.PutObject(r.Context(), bucket, key, tmp, n, "")
	}
	if err != nil {
		slog.Error("object replica", "bucket", bucket, "key", key, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !etagEqual(etag, md5hex) {
		if versionID != "" {
			_ = s.backend.DeleteObjectVersion(r.Context(), bucket, key, versionID)
		} else {
			_ = s.backend.DeleteObject(r.Context(), bucket, key)
		}
		w.WriteHeader(http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func spoolReplica(r io.Reader) (tmp *os.File, shaHex, md5hex string, n int64, err error) {
	tmp, err = os.CreateTemp("", "ds-object-replica-*")
	if err != nil {
		return nil, "", "", 0, err
	}
	h := sha256.New()
	m := md5.New()
	n, err = io.Copy(io.MultiWriter(tmp, h, m), r)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, "", "", 0, err
	}
	return tmp, hex.EncodeToString(h.Sum(nil)), hex.EncodeToString(m.Sum(nil)), n, nil
}

func (s *Server) serveObjectReplica(w http.ResponseWriter, r *http.Request, rec metadata.ObjectRecord) {
	rc, _, err := s.backend.GetObjectVersion(r.Context(), rec.Bucket, rec.Key, rec.VersionID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slog.Error("object replica read", "bucket", rec.Bucket, "key", rec.Key, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rc.Close()
	tmp, shaHex, md5hex, n, err := spoolReplica(rc)
	if err != nil {
		slog.Error("object replica spool", "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	_ = shaHex
	if !replicaPayloadOK(rec, n, md5hex) {
		w.WriteHeader(http.StatusConflict)
		return
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(n, 10))
	w.Header().Set("ETag", rec.ETag)
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = io.Copy(w, tmp)
}

// replicateStoredObject copies the file just written to every other node that
// advertised an address. A node that does not answer is skipped: a killed
// leader stays in the heartbeat table until its row ages out.
func (s *Server) replicateStoredObject(ctx context.Context, bucket, key, versionID string) {
	if s == nil || s.haLeader == nil || !s.haLeader.Enabled() || s.cfg.JWTSecret == "" {
		return
	}
	peers := s.replicaPeers()
	if len(peers) == 0 {
		return
	}
	rc, _, err := s.backend.GetObjectVersion(ctx, bucket, key, versionID)
	if err != nil {
		slog.Error("object replica read", "bucket", bucket, "key", key, "err", err)
		return
	}
	defer rc.Close()
	tmp, err := os.CreateTemp("", "ds-object-replica-*")
	if err != nil {
		slog.Error("object replica temp", "err", err)
		return
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	h := sha256.New()
	m := md5.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h, m), rc); err != nil {
		slog.Error("object replica spool", "bucket", bucket, "key", key, "err", err)
		return
	}
	st, err := tmp.Stat()
	if err != nil {
		return
	}
	bodyHash := hex.EncodeToString(h.Sum(nil))
	md5hex := hex.EncodeToString(m.Sum(nil))
	for _, peer := range peers {
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return
		}
		if err := s.pushObjectReplica(ctx, peer, bucket, key, versionID, tmp, st.Size(), bodyHash, md5hex); err != nil {
			slog.Error("object replica push", "peer", peer.Addr, "bucket", bucket, "key", key, "err", err)
		}
	}
}

type replicaPeer struct {
	ID   string
	Addr string
}

func (s *Server) replicaPeers() []replicaPeer {
	beats, err := s.meta.ListNodeHeartbeats(time.Now().Add(-2 * time.Minute))
	if err != nil {
		return nil
	}
	self := s.clusterNodeID()
	var out []replicaPeer
	seen := map[string]bool{}
	for _, b := range beats {
		addr := strings.TrimSpace(b.Address)
		if b.NodeID == self || addr == "" || seen[b.NodeID] {
			continue
		}
		seen[b.NodeID] = true
		out = append(out, replicaPeer{ID: b.NodeID, Addr: replicaPeerAddr(addr)})
	}
	return out
}

// replicaPeerAddr dials the internal listener. The public port and the load
// balancer do not serve this API. An empty internal port keeps the advertised
// URL, which then answers 404.
func replicaPeerAddr(public string) string {
	port := internalReplicaPort()
	if port == "" || strings.TrimSpace(public) == "" {
		return public
	}
	raw := public
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return public
	}
	u.Host = net.JoinHostPort(u.Hostname(), port)
	return u.String()
}

// DefaultInternalAddr is the peer listener address used when HA is on and
// STORAGE_INTERNAL_ADDR is not set. Without a derived default a replica
// answered 404 on /api/v1/ha/object-replica, because the public port does not
// serve that API.
const DefaultInternalAddr = ":9002"

// InternalAddr returns the address of the peer listener. STORAGE_INTERNAL_ADDR
// wins. When it is empty and STORAGE_HA_ENABLED is on, the listener is
// DefaultInternalAddr. With HA off and no value there is no peer listener.
func InternalAddr() string {
	if addr := strings.TrimSpace(os.Getenv("STORAGE_INTERNAL_ADDR")); addr != "" {
		return addr
	}
	if ha.ConfigFromEnv().Enabled {
		return DefaultInternalAddr
	}
	return ""
}

func internalReplicaPort() string {
	if p := strings.TrimSpace(os.Getenv("STORAGE_INTERNAL_PORT")); p != "" {
		return strings.TrimPrefix(p, ":")
	}
	addr := InternalAddr()
	if addr == "" {
		return ""
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return ""
	}
	return port
}

var replicaClient = &http.Client{
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: 1 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 5 * time.Second,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
	},
}

// fetchStoredObject copies a file that this disk does not have from a peer
// that does. Public share download and other local reads stay on this node.
func (s *Server) fetchStoredObject(ctx context.Context, bucket, key, versionID string) error {
	if s == nil || s.haLeader == nil || !s.haLeader.Enabled() || s.cfg.JWTSecret == "" {
		return storage.ErrNotFound
	}
	return s.pullFromPeers(ctx, s.replicaPeers(), bucket, key, versionID)
}

func (s *Server) pullFromPeers(ctx context.Context, peers []replicaPeer, bucket, key, versionID string) error {
	if len(peers) == 0 {
		return storage.ErrNotFound
	}
	var last error = storage.ErrNotFound
	for _, peer := range peers {
		err := s.pullObjectReplica(ctx, peer, bucket, key, versionID)
		if err == nil {
			return nil
		}
		last = err
		slog.Error("object replica pull", "peer", peer.Addr, "bucket", bucket, "key", key, "err", err)
	}
	return last
}

func (s *Server) pullObjectReplica(ctx context.Context, peer replicaPeer, bucket, key, versionID string) error {
	addr := peer.Addr
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	q := url.Values{}
	q.Set("bucket", bucket)
	q.Set("key", key)
	q.Set("version", versionID)
	endpoint := strings.TrimRight(addr, "/") + replicaPath + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	s.signReplica(req, peer.ID, bucket, key, versionID, emptyBodySHA())
	resp, err := replicaClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, resp.Body)
		return storage.ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return errStatus(resp.StatusCode)
	}
	tmp, _, md5hex, n, err := spoolReplica(resp.Body)
	if err != nil {
		return err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	rec, err := s.replicaMeta(bucket, key, versionID)
	if err != nil {
		return err
	}
	if !replicaPayloadOK(rec, n, md5hex) {
		return errStatus(http.StatusConflict)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var etag string
	if versionID != "" {
		etag, err = s.backend.PutObjectVersion(ctx, bucket, key, versionID, tmp, n, "")
	} else {
		etag, err = s.backend.PutObject(ctx, bucket, key, tmp, n, "")
	}
	if err != nil {
		return err
	}
	if !etagEqual(etag, md5hex) {
		return errStatus(http.StatusConflict)
	}
	return nil
}

func (s *Server) pushObjectReplica(ctx context.Context, peer replicaPeer, bucket, key, versionID string, body io.Reader, size int64, bodyHash, md5hex string) error {
	addr := peer.Addr
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	q := url.Values{}
	q.Set("bucket", bucket)
	q.Set("key", key)
	q.Set("version", versionID)
	endpoint := strings.TrimRight(addr, "/") + replicaPath + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	s.signReplica(req, peer.ID, bucket, key, versionID, bodyHash)
	_ = md5hex
	resp, err := replicaClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return errStatus(resp.StatusCode)
	}
	return nil
}

type errStatus int

func (e errStatus) Error() string {
	return "object replica HTTP " + http.StatusText(int(e))
}

// proxyObjectRead sends a follower's object GET or HEAD to the leader.
// The leader has the current bytes. The original Host is kept for SigV4.
// A failed dial falls through so the follower can serve its own copy.
func (s *Server) proxyObjectRead(w http.ResponseWriter, r *http.Request) bool {
	if s.haLeader == nil || !s.haLeader.Enabled() || s.haLeader.IsLeader(r.Context()) {
		return false
	}
	if r.Header.Get("X-DataSafe-Forwarded") != "" || !isHAObjectRead(r) {
		return false
	}
	return s.forwardToLeader(w, r)
}

func isHAObjectRead(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	p := strings.Trim(r.URL.Path, "/")
	return p != "" && strings.Contains(p, "/")
}
