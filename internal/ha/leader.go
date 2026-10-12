package ha

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	ErrNotLeader     = errors.New("ha: not cluster leader")
	ErrLeaderHeld    = errors.New("ha: leader lock held by another node")
	ErrHANotPostgres = errors.New("ha: leader lock requires postgres metadata backend")
)

// LeaderStore persists HA leader leases.
type LeaderStore interface {
	TryAcquireLeader(ctx context.Context, holderID string, ttl time.Duration) (bool, error)
	RenewLeader(ctx context.Context, holderID string, ttl time.Duration) (bool, error)
	ReleaseLeader(ctx context.Context, holderID string) error
	CurrentLeader(ctx context.Context) (holderID string, expiresAt time.Time, ok bool, err error)
}

// Config from environment.
type Config struct {
	Enabled    bool
	NodeID     string
	LeaderTTL  time.Duration
	RenewEvery time.Duration
}

func ConfigFromEnv() Config {
	cfg := Config{
		Enabled: strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_HA_ENABLED"))) == "true" ||
			os.Getenv("STORAGE_HA_ENABLED") == "1",
		NodeID: strings.TrimSpace(os.Getenv("STORAGE_NODE_ID")),
	}
	if cfg.NodeID == "" {
		h, _ := os.Hostname()
		cfg.NodeID = h
	}
	cfg.LeaderTTL = 30 * time.Second
	if v := strings.TrimSpace(os.Getenv("STORAGE_HA_LEADER_TTL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.LeaderTTL = d
		}
	}
	cfg.RenewEvery = cfg.LeaderTTL / 3
	if cfg.RenewEvery < 5*time.Second {
		cfg.RenewEvery = 10 * time.Second
	}
	return cfg
}

// Leader coordinates single-writer metadata HA.
type Leader struct {
	cfg   Config
	store LeaderStore

	mu   sync.Mutex
	lost bool
}

func New(cfg Config, store LeaderStore) *Leader {
	return &Leader{cfg: cfg, store: store}
}

func (l *Leader) Enabled() bool { return l.cfg.Enabled }

func (l *Leader) NodeID() string { return l.cfg.NodeID }

// Start acquires the lease and runs renew loop until ctx cancelled.
func (l *Leader) Start(ctx context.Context) error {
	if !l.cfg.Enabled {
		return nil
	}
	if l.store == nil {
		return ErrHANotPostgres
	}
	ok, err := l.store.TryAcquireLeader(ctx, l.cfg.NodeID, l.cfg.LeaderTTL)
	if err != nil {
		return err
	}
	if !ok {
		holder, exp, _, _ := l.store.CurrentLeader(ctx)
		return fmt.Errorf("%w (holder=%s expires=%s)", ErrLeaderHeld, holder, exp.UTC())
	}
	ticker := time.NewTicker(l.cfg.RenewEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = l.store.ReleaseLeader(context.Background(), l.cfg.NodeID)
			return nil
		case <-ticker.C:
			ok, err := l.store.RenewLeader(ctx, l.cfg.NodeID, l.cfg.LeaderTTL)
			if err != nil || !ok {
				// A fenced primary (read-only Postgres, or a lease that is no
				// longer ours) must refuse writes immediately. Waiting out the
				// old TTL would leave a split brain. The process does not
				// acquire again; start it on the new primary to lead.
				l.markLost()
				if err != nil {
					return err
				}
				return ErrNotLeader
			}
		}
	}
}

// Resign drops this process's lease claim immediately. The event dispatcher
// calls it when an outbox write hits a read-only database, so delivery stops
// without waiting for the next renew.
func (l *Leader) Resign() {
	if l == nil {
		return
	}
	l.markLost()
}

// markLost drops this process's claim. leaderGuard then rejects writes
// even if the previous lease row has not expired yet.
func (l *Leader) markLost() {
	l.mu.Lock()
	l.lost = true
	l.mu.Unlock()
}

// IsLeader returns true when HA disabled or this node holds a valid lease.
// It is false after a failed renew, including when the metadata primary has
// been fenced read-only.
func (l *Leader) IsLeader(ctx context.Context) bool {
	if !l.cfg.Enabled {
		return true
	}
	l.mu.Lock()
	lost := l.lost
	l.mu.Unlock()
	if lost {
		return false
	}
	holder, exp, ok, err := l.store.CurrentLeader(ctx)
	if err != nil || !ok {
		return false
	}
	return holder == l.cfg.NodeID && time.Now().UTC().Before(exp)
}

func (l *Leader) RequireLeader(ctx context.Context) error {
	if l.IsLeader(ctx) {
		return nil
	}
	return ErrNotLeader
}
