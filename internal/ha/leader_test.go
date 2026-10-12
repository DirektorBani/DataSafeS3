package ha_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/ha"
)

type memStore struct {
	mu     sync.Mutex
	holder string
	exp    time.Time
}

func (m *memStore) TryAcquireLeader(_ context.Context, holderID string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	if m.holder != "" && m.exp.After(now) && m.holder != holderID {
		return false, nil
	}
	m.holder = holderID
	m.exp = now.Add(ttl)
	return true, nil
}

func (m *memStore) RenewLeader(_ context.Context, holderID string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holder != holderID {
		return false, nil
	}
	m.exp = time.Now().UTC().Add(ttl)
	return true, nil
}

func (m *memStore) ReleaseLeader(_ context.Context, holderID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holder == holderID {
		m.holder = ""
	}
	return nil
}

func (m *memStore) CurrentLeader(_ context.Context) (string, time.Time, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.holder == "" || time.Now().UTC().After(m.exp) {
		return "", time.Time{}, false, nil
	}
	return m.holder, m.exp, true, nil
}

func TestLeaderOnAcquiredFiresOnce(t *testing.T) {
	store := &memStore{}
	l := ha.New(ha.Config{Enabled: true, NodeID: "node-a", LeaderTTL: time.Second, RenewEvery: 30 * time.Millisecond}, store)
	var n atomic.Int32
	l.SetOnAcquired(func() { n.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- l.Start(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for n.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n.Load() != 1 {
		t.Fatalf("onAcquired calls %d", n.Load())
	}
	time.Sleep(100 * time.Millisecond)
	if n.Load() != 1 {
		t.Fatalf("renew called onAcquired again: %d", n.Load())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leader did not stop")
	}
}

func TestLeaderAcquireAndRenew(t *testing.T) {
	store := &memStore{}
	cfg := ha.Config{Enabled: true, NodeID: "node-a", LeaderTTL: time.Minute, RenewEvery: time.Second}
	l := ha.New(cfg, store)
	ok, err := store.TryAcquireLeader(context.Background(), "node-a", time.Minute)
	if err != nil || !ok {
		t.Fatal("acquire")
	}
	if !l.IsLeader(context.Background()) {
		t.Fatal("should be leader")
	}
	ok, _ = store.TryAcquireLeader(context.Background(), "node-b", time.Minute)
	if ok {
		t.Fatal("second node should not steal active lease")
	}
}

type failRenew struct {
	memStore
}

func (f *failRenew) RenewLeader(context.Context, string, time.Duration) (bool, error) {
	return false, fmt.Errorf("cannot execute UPDATE in a read-only transaction")
}

func TestLeaderRenewFailureRefusesWritesImmediately(t *testing.T) {
	store := &failRenew{}
	cfg := ha.Config{Enabled: true, NodeID: "node-a", LeaderTTL: time.Minute, RenewEvery: 20 * time.Millisecond}
	l := ha.New(cfg, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errc := make(chan error, 1)
	go func() { errc <- l.Start(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for l.IsLeader(context.Background()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if l.IsLeader(context.Background()) {
		t.Fatal("fenced leader still reports the lease")
	}
	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("renew failure returned nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("leader loop did not stop")
	}
}

func TestLeaderRaceConcurrentAcquire(t *testing.T) {
	store := &memStore{}
	cfg := ha.Config{Enabled: true, NodeID: "node-a", LeaderTTL: time.Minute, RenewEvery: time.Second}
	_ = ha.New(cfg, store)

	const nodes = 8
	wins := make(chan string, nodes)
	for i := 0; i < nodes; i++ {
		id := fmt.Sprintf("node-%d", i)
		go func(holder string) {
			ok, err := store.TryAcquireLeader(context.Background(), holder, time.Minute)
			if err == nil && ok {
				wins <- holder
			}
		}(id)
	}

	var winner string
	select {
	case winner = <-wins:
	case <-time.After(2 * time.Second):
		t.Fatal("no node acquired leader")
	}

	// Only one winner; others must not steal while lease active.
	for i := 0; i < nodes-1; i++ {
		select {
		case extra := <-wins:
			t.Fatalf("multiple winners: %s and %s", winner, extra)
		case <-time.After(50 * time.Millisecond):
		}
	}
	other := "node-other"
	ok, _ := store.TryAcquireLeader(context.Background(), other, time.Minute)
	if ok {
		t.Fatal("concurrent acquire should not steal active lease")
	}
}

func TestLeaderRenewIsAtMostOneThirdOfLease(t *testing.T) {
	t.Setenv("STORAGE_HA_LEADER_TTL", "10s")
	cfg := ha.ConfigFromEnv()
	if cfg.LeaderTTL != 10*time.Second {
		t.Fatalf("ttl %s", cfg.LeaderTTL)
	}
	if cfg.RenewEvery != cfg.LeaderTTL/3 {
		t.Fatalf("renew %s, want %s", cfg.RenewEvery, cfg.LeaderTTL/3)
	}
	if cfg.RenewEvery > cfg.LeaderTTL/3 {
		t.Fatalf("renew %s exceeds lease/3", cfg.RenewEvery)
	}

	t.Setenv("STORAGE_HA_LEADER_TTL", "14s")
	cfg = ha.ConfigFromEnv()
	if cfg.LeaderTTL != 14*time.Second {
		t.Fatalf("ttl %s", cfg.LeaderTTL)
	}
	if cfg.RenewEvery != 14*time.Second/3 {
		t.Fatalf("renew %s, want %s", cfg.RenewEvery, 14*time.Second/3)
	}
}

// expireLeader stops renewing for node-a without clearing the row, so the
// lease stays until TTL. A follower must wait that out. node-a must not lead again.
type expireLeader struct {
	memStore
	flag sync.Mutex
	dead bool
}

func (e *expireLeader) RenewLeader(ctx context.Context, holderID string, ttl time.Duration) (bool, error) {
	e.flag.Lock()
	dead := e.dead
	e.flag.Unlock()
	if dead && holderID == "node-a" {
		return false, nil
	}
	return e.memStore.RenewLeader(ctx, holderID, ttl)
}

func TestFollowerTakesOverAfterLeaseExpires(t *testing.T) {
	store := &expireLeader{}
	ttl := time.Second
	renew := 40 * time.Millisecond
	leader := ha.New(ha.Config{Enabled: true, NodeID: "node-a", LeaderTTL: ttl, RenewEvery: renew}, store)
	follower := ha.New(ha.Config{Enabled: true, NodeID: "node-b", LeaderTTL: ttl, RenewEvery: renew}, store)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	leaderErr := make(chan error, 1)
	go func() { leaderErr <- leader.Start(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for !leader.IsLeader(context.Background()) && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !leader.IsLeader(context.Background()) {
		t.Fatal("leader did not acquire")
	}

	followErr := make(chan error, 1)
	go func() { followErr <- follower.Start(ctx) }()
	time.Sleep(30 * time.Millisecond)
	if follower.IsLeader(context.Background()) {
		t.Fatal("follower stole a live lease")
	}

	store.flag.Lock()
	store.dead = true
	store.flag.Unlock()

	select {
	case err := <-leaderErr:
		if !errors.Is(err, ha.ErrNotLeader) {
			t.Fatalf("leader exit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fenced leader did not stop")
	}
	if leader.IsLeader(context.Background()) {
		t.Fatal("fenced leader still reports the lease")
	}
	if follower.IsLeader(context.Background()) {
		t.Fatal("follower acquired before the lease expired")
	}

	deadline = time.Now().Add(2 * time.Second)
	for !follower.IsLeader(context.Background()) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !follower.IsLeader(context.Background()) {
		t.Fatal("follower did not take over after the lease expired")
	}
	if leader.IsLeader(context.Background()) {
		t.Fatal("fenced leader acquired again")
	}
	cancel()
	select {
	case err := <-followErr:
		if err != nil {
			t.Fatalf("follower stop: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("follower did not stop")
	}
}
