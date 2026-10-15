package vaulttransit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DirektorBani/datasafe/internal/kms"
	"github.com/DirektorBani/datasafe/internal/observability"
)

// Provider is the in-process Vault Transit key source.
// Open returns nil when the provider is not vault-transit or the address is empty.
type Provider struct {
	client *Client
	cache  *dekCache
	poll   time.Duration
	tout   time.Duration

	mu        sync.Mutex
	gens      map[string]int64
	available bool
	probed    bool
	lastErr   string
	known     int // 0 unknown, 1 up, 2 down
}

// Open builds a client. It does not dial Vault. A disabled config returns nil, nil.
func Open(cfg Config) (*Provider, error) {
	cfg.applyDefaults()
	if !cfg.Enabled() {
		return nil, nil
	}
	client, err := newClient(cfg)
	if err != nil || client == nil {
		return nil, err
	}
	return &Provider{
		client: client,
		cache:  newDEKCache(cfg.CacheTTL, cfg.CacheMax),
		poll:   cfg.PollInterval,
		tout:   cfg.Timeout,
		gens:   map[string]int64{},
	}, nil
}

func (p *Provider) Name() string { return kms.ProviderVault }

func (p *Provider) Timeout() time.Duration {
	if p == nil || p.tout <= 0 {
		return defaultTimeout
	}
	return p.tout
}

func (p *Provider) PollInterval() time.Duration {
	if p == nil || p.poll <= 0 {
		return defaultPoll
	}
	return p.poll
}

func (p *Provider) Available() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.available
}

func (p *Provider) Probed() bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.probed
}

func (p *Provider) LastError() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr
}

// Mark records a health transition. The result is an activity action or "".
func (p *Provider) Mark(up bool, code string) string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.probed = true
	p.available = up
	if up {
		p.lastErr = ""
		if p.known == 2 {
			p.known = 1
			return "kms_recovered"
		}
		p.known = 1
		return ""
	}
	p.lastErr = code
	if p.known != 2 {
		p.known = 2
		return "kms_unavailable"
	}
	return ""
}

// NoteGenerations drops cached DEKs whose generation is no longer current.
func (p *Provider) NoteGenerations(gens map[string]int64) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.gens == nil {
		p.gens = map[string]int64{}
	}
	for id, gen := range gens {
		if prev, ok := p.gens[id]; ok && prev != gen {
			p.cache.dropKey(id, gen)
		}
		p.gens[id] = gen
	}
}

func (p *Provider) observe(op string, err error, start time.Time) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	observability.ObserveKMS(op, kms.ProviderVault, result, time.Since(start))
}

func (p *Provider) Wrap(ctx context.Context, keyID string, dek []byte) ([]byte, int, error) {
	start := time.Now()
	if err := p.client.EnsureKey(ctx, keyID); err != nil {
		p.observe("wrap", err, start)
		return nil, 0, kms.ErrUnavailable
	}
	ct, ver, err := p.client.Encrypt(ctx, keyID, dek)
	p.observe("wrap", err, start)
	if err != nil || ct == "" {
		return nil, 0, kms.ErrUnavailable
	}
	return []byte(ct), ver, nil
}

func (p *Provider) Unwrap(ctx context.Context, keyID string, version int, wrapped []byte) ([]byte, error) {
	p.mu.Lock()
	gen := p.gens[keyID]
	p.mu.Unlock()
	return p.UnwrapDEK(ctx, keyID, version, gen, wrapped)
}

func (p *Provider) UnwrapDEK(ctx context.Context, keyID string, version int, generation int64, wrapped []byte) ([]byte, error) {
	if dek, ok := p.cache.get(keyID, version, generation, wrapped); ok {
		observability.KMSCacheHit()
		return dek, nil
	}
	observability.KMSCacheMiss()
	start := time.Now()
	plain, err := p.client.Decrypt(ctx, keyID, string(wrapped))
	p.observe("unwrap", err, start)
	if err != nil || len(plain) == 0 {
		return nil, kms.ErrUnavailable
	}
	p.cache.put(keyID, version, generation, wrapped, plain)
	return append([]byte(nil), plain...), nil
}

func (p *Provider) Rewrap(ctx context.Context, keyID string, version int, wrapped []byte) ([]byte, int, error) {
	start := time.Now()
	ct, ver, err := p.client.Rewrap(ctx, keyID, string(wrapped))
	p.observe("rewrap", err, start)
	if err != nil || ct == "" {
		return nil, 0, kms.ErrUnavailable
	}
	if ver < 1 {
		ver = version + 1
	}
	return []byte(ct), ver, nil
}

func (p *Provider) Rotate(ctx context.Context, keyID string) (int, error) {
	start := time.Now()
	ver, err := p.client.Rotate(ctx, keyID)
	p.observe("rotate", err, start)
	if err != nil {
		return 0, kms.ErrUnavailable
	}
	return ver, nil
}

func (p *Provider) Destroy(ctx context.Context, keyID string) error {
	start := time.Now()
	err := p.client.DeleteKey(ctx, keyID)
	p.observe("destroy", err, start)
	if err != nil {
		return fmt.Errorf("vault delete: %s", err.Error())
	}
	return nil
}

func (p *Provider) Health(ctx context.Context) error {
	start := time.Now()
	err := p.client.Health(ctx)
	p.observe("health", err, start)
	return err
}

// Test logs in and encrypts a fixed string on datasafe-connection-test.
func (p *Provider) Test(ctx context.Context) error {
	start := time.Now()
	err := p.client.TestLoginEncrypt(ctx)
	p.observe("test", err, start)
	return Classify(err)
}
