package api

import (
	"context"
	"crypto/hmac"
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
	"time"

	"github.com/DirektorBani/datasafe/internal/storage"
)

// replicaMAC authenticates a copy of stored object bytes between HA nodes.
// The secret is the JWT secret every node in the lab already shares.
func replicaMAC(secret, bucket, key, versionID string) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(bucket))
	_, _ = m.Write([]byte{0})
	_, _ = m.Write([]byte(key))
	_, _ = m.Write([]byte{0})
	_, _ = m.Write([]byte(versionID))
	return hex.EncodeToString(m.Sum(nil))
}

func replicaMACOK(secret, bucket, key, versionID, got string) bool {
	if secret == "" || got == "" {
		return false
	}
	want := replicaMAC(secret, bucket, key, versionID)
	a, errA := hex.DecodeString(want)
	b, errB := hex.DecodeString(got)
	if errA != nil || errB != nil || len(a) != len(b) {
		return false
	}
	return hmac.Equal(a, b)
}

// handleObjectReplica stores or returns bytes already written on a peer.
// It does not update metadata and it does not go through the leader proxy.
func (s *Server) handleObjectReplica(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	bucket := r.URL.Query().Get("bucket")
	key := r.URL.Query().Get("key")
	versionID := r.URL.Query().Get("version")
	if bucket == "" || key == "" || !replicaMACOK(s.cfg.JWTSecret, bucket, key, versionID, r.Header.Get("X-DataSafe-Replica")) {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		s.serveObjectReplica(w, r, bucket, key, versionID)
		return
	}
	var err error
	if versionID != "" {
		_, err = s.backend.PutObjectVersion(r.Context(), bucket, key, versionID, r.Body, r.ContentLength, "")
	} else {
		_, err = s.backend.PutObject(r.Context(), bucket, key, r.Body, r.ContentLength, "")
	}
	if err != nil {
		slog.Error("object replica", "bucket", bucket, "key", key, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serveObjectReplica(w http.ResponseWriter, r *http.Request, bucket, key, versionID string) {
	rc, info, err := s.backend.GetObjectVersion(r.Context(), bucket, key, versionID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		slog.Error("object replica read", "bucket", bucket, "key", key, "err", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = io.Copy(w, rc)
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
	if _, err := io.Copy(tmp, rc); err != nil {
		slog.Error("object replica spool", "bucket", bucket, "key", key, "err", err)
		return
	}
	st, err := tmp.Stat()
	if err != nil {
		return
	}
	for _, addr := range peers {
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return
		}
		if err := s.pushObjectReplica(ctx, addr, bucket, key, versionID, tmp, st.Size()); err != nil {
			slog.Error("object replica push", "peer", addr, "bucket", bucket, "key", key, "err", err)
		}
	}
}

func (s *Server) replicaPeers() []string {
	beats, err := s.meta.ListNodeHeartbeats(time.Now().Add(-2 * time.Minute))
	if err != nil {
		return nil
	}
	self := s.clusterNodeID()
	var out []string
	seen := map[string]bool{}
	for _, b := range beats {
		addr := strings.TrimSpace(b.Address)
		if b.NodeID == self || addr == "" || seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, addr)
	}
	return out
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

func (s *Server) pullFromPeers(ctx context.Context, peers []string, bucket, key, versionID string) error {
	if len(peers) == 0 {
		return storage.ErrNotFound
	}
	var last error = storage.ErrNotFound
	for _, addr := range peers {
		err := s.pullObjectReplica(ctx, addr, bucket, key, versionID)
		if err == nil {
			return nil
		}
		last = err
		slog.Error("object replica pull", "peer", addr, "bucket", bucket, "key", key, "err", err)
	}
	return last
}

func (s *Server) pullObjectReplica(ctx context.Context, addr, bucket, key, versionID string) error {
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	q := url.Values{}
	q.Set("bucket", bucket)
	q.Set("key", key)
	q.Set("version", versionID)
	endpoint := strings.TrimRight(addr, "/") + "/api/v1/ha/object-replica?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-DataSafe-Replica", replicaMAC(s.cfg.JWTSecret, bucket, key, versionID))
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
	size := resp.ContentLength
	body := io.Reader(resp.Body)
	if size < 0 {
		tmp, terr := os.CreateTemp("", "ds-object-pull-*")
		if terr != nil {
			return terr
		}
		defer func() {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}()
		n, cerr := io.Copy(tmp, resp.Body)
		if cerr != nil {
			return cerr
		}
		if _, serr := tmp.Seek(0, io.SeekStart); serr != nil {
			return serr
		}
		size = n
		body = tmp
	}
	if versionID != "" {
		_, err = s.backend.PutObjectVersion(ctx, bucket, key, versionID, body, size, "")
	} else {
		_, err = s.backend.PutObject(ctx, bucket, key, body, size, "")
	}
	return err
}

func (s *Server) pushObjectReplica(ctx context.Context, addr, bucket, key, versionID string, body io.Reader, size int64) error {
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	q := url.Values{}
	q.Set("bucket", bucket)
	q.Set("key", key)
	q.Set("version", versionID)
	endpoint := strings.TrimRight(addr, "/") + "/api/v1/ha/object-replica?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("X-DataSafe-Replica", replicaMAC(s.cfg.JWTSecret, bucket, key, versionID))
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
