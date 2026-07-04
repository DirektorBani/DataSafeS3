package ha_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DirektorBani/datasafe/internal/ha"
)

type memStore struct {
	holder string
	exp    time.Time
}

func (m *memStore) TryAcquireLeader(_ context.Context, holderID string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	if m.holder != "" && m.exp.After(now) && m.holder != holderID {
		return false, nil
	}
	m.holder = holderID
	m.exp = now.Add(ttl)
	return true, nil
}

func (m *memStore) RenewLeader(_ context.Context, holderID string, ttl time.Duration) (bool, error) {
	if m.holder != holderID {
		return false, nil
	}
	m.exp = time.Now().UTC().Add(ttl)
	return true, nil
}

func (m *memStore) ReleaseLeader(_ context.Context, holderID string) error {
	if m.holder == holderID {
		m.holder = ""
	}
	return nil
}

func (m *memStore) CurrentLeader(_ context.Context) (string, time.Time, bool, error) {
	if m.holder == "" || time.Now().UTC().After(m.exp) {
		return "", time.Time{}, false, nil
	}
	return m.holder, m.exp, true, nil
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
