package events

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultRetention     = 24 * time.Hour
	DefaultDeadRetention = 168 * time.Hour
	DefaultMaxPending    = 100000
	DefaultMaxAttempts   = 20

	// PollInterval is how often the dispatcher looks for due rows.
	// Not an environment variable: tests call ProcessOnce directly.
	PollInterval = time.Second
	// PurgeEvery is how often delivered and dead rows are deleted.
	PurgeEvery = time.Minute

	baseBackoff = time.Second
	maxBackoff  = 5 * time.Minute
)

// Config is the outbox dispatcher configuration.
// Every field has a default. None of the STORAGE_EVENTS_* variables are required.
type Config struct {
	Retention     time.Duration
	DeadRetention time.Duration
	MaxPending    int
	MaxAttempts   int
}

// ConfigFromEnv reads STORAGE_EVENTS_RETENTION, STORAGE_EVENTS_DEAD_RETENTION,
// STORAGE_EVENTS_MAX_PENDING, and STORAGE_EVENTS_MAX_ATTEMPTS.
// Empty or invalid values keep the defaults. MaxPending 0 disables the cap.
func ConfigFromEnv() Config {
	cfg := Config{
		Retention:     DefaultRetention,
		DeadRetention: DefaultDeadRetention,
		MaxPending:    DefaultMaxPending,
		MaxAttempts:   DefaultMaxAttempts,
	}
	if d, ok := envDuration("STORAGE_EVENTS_RETENTION"); ok {
		cfg.Retention = d
	}
	if d, ok := envDuration("STORAGE_EVENTS_DEAD_RETENTION"); ok {
		cfg.DeadRetention = d
	}
	if n, ok := envNonNegInt("STORAGE_EVENTS_MAX_PENDING"); ok {
		cfg.MaxPending = n
	}
	if n, ok := envNonNegInt("STORAGE_EVENTS_MAX_ATTEMPTS"); ok && n >= 1 {
		cfg.MaxAttempts = n
	}
	return cfg
}

func envDuration(key string) (time.Duration, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, false
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, false
	}
	return d, true
}

func envNonNegInt(key string) (int, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}
