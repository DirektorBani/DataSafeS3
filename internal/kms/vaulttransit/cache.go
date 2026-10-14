package vaulttransit

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"sync"
	"time"
)

type dekEntry struct {
	keyID string
	gen   int64
	dek   []byte
	exp   time.Time
}

// dekCache is an LRU of unwrapped DEKs. The key includes key id, version, and generation.
type dekCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	max   int
	order []string
	items map[string]dekEntry
}

func newDEKCache(ttl time.Duration, max int) *dekCache {
	if max < 1 {
		max = defaultCacheMax
	}
	if ttl <= 0 {
		ttl = defaultCacheTTL
	}
	return &dekCache{ttl: ttl, max: max, items: map[string]dekEntry{}}
}

func dekCacheKey(keyID string, version int, gen int64, wrapped []byte) string {
	sum := sha256.Sum256(wrapped)
	return keyID + "|" + strconv.Itoa(version) + "|" + strconv.FormatInt(gen, 10) + "|" + hex.EncodeToString(sum[:8])
}

func (c *dekCache) get(keyID string, version int, gen int64, wrapped []byte) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	k := dekCacheKey(keyID, version, gen, wrapped)
	c.mu.Lock()
	defer c.mu.Unlock()
	ent, ok := c.items[k]
	if !ok {
		return nil, false
	}
	if time.Now().After(ent.exp) {
		c.deleteLocked(k)
		return nil, false
	}
	c.touchLocked(k)
	return append([]byte(nil), ent.dek...), true
}

func (c *dekCache) put(keyID string, version int, gen int64, wrapped, dek []byte) {
	if c == nil || len(dek) == 0 {
		return
	}
	k := dekCacheKey(keyID, version, gen, wrapped)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[k]; ok {
		c.deleteLocked(k)
	}
	for len(c.items) >= c.max && len(c.order) > 0 {
		c.deleteLocked(c.order[0])
	}
	c.items[k] = dekEntry{
		keyID: keyID,
		gen:   gen,
		dek:   append([]byte(nil), dek...),
		exp:   time.Now().Add(c.ttl),
	}
	c.order = append(c.order, k)
}

// dropKey removes entries for keyID whose generation is not current.
func (c *dekCache) dropKey(keyID string, gen int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, ent := range c.items {
		if ent.keyID == keyID && ent.gen != gen {
			c.deleteLocked(k)
		}
	}
}

func (c *dekCache) touchLocked(k string) {
	for i, cur := range c.order {
		if cur == k {
			c.order = append(c.order[:i], c.order[i+1:]...)
			c.order = append(c.order, k)
			return
		}
	}
}

func (c *dekCache) deleteLocked(k string) {
	delete(c.items, k)
	for i, cur := range c.order {
		if cur == k {
			c.order = append(c.order[:i], c.order[i+1:]...)
			return
		}
	}
}
