package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateEntry
	limit   int
	window  time.Duration
}

type rateEntry struct {
	count    int
	windowAt time.Time
}

func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		entries: make(map[string]*rateEntry),
		limit:   limit,
		window:  window,
	}
}

func (rl *ipRateLimiter) allow(ip string) (bool, time.Duration) {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.entries[ip]
	if !ok || now.Sub(e.windowAt) >= rl.window {
		rl.entries[ip] = &rateEntry{count: 1, windowAt: now}
		return true, 0
	}
	if e.count >= rl.limit {
		retry := rl.window - now.Sub(e.windowAt)
		if retry < 0 {
			retry = 0
		}
		return false, retry
	}
	e.count++
	return true, 0
}

func rateLimitByIP(rl *ipRateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		ok, retry := rl.allow(ip)
		if !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
			return
		}
		next(w, r)
	}
}
