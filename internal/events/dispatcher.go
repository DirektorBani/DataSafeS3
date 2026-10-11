package events

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"strings"
	"sync"
	"time"
)

// ErrStoreFenced means a metadata write was refused because this process
// must not keep delivering. The dispatcher stops and drops the leader lease.
// A row already published and then not marked delivered can be sent once
// more by the next leader. It is not published again by this process.
var ErrStoreFenced = errors.New("events: store refused a leader write")

// StoreFenced reports a read-only transaction or an explicit not-leader
// failure from the metadata store.
func StoreFenced(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrStoreFenced) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "SQLSTATE 25006") || strings.Contains(msg, "read-only transaction") {
		return true
	}
	return strings.Contains(msg, "not cluster leader")
}

// LeaderFunc reports whether this process should deliver.
// Nil, or HA disabled, means always deliver. In HA only the leader returns true.
type LeaderFunc func(ctx context.Context) bool

// WaitingForConnection is the degraded reason before the first successful
// probe or publish. Package kafkasink uses the same sentence. A real probe
// or publish error replaces it. Activity does not record this sentence.
const WaitingForConnection = "waiting for first successful connection"

// Observer receives delivery signals. Nil funcs are skipped.
// Wired to Prometheus and Activity by the API process.
type Observer struct {
	Pending   func(n int)
	Delivered func(sink string)
	Failed    func(sink, reason string)
	Dead      func(sink string)
	Lag       func(sink string, seconds float64)
	// Degraded is 1 while status is degraded, including before the first
	// successful connection, and 0 when the destination is connected.
	Degraded func(sink string, degraded bool)
	// SinkRemoved drops the degraded series after Unregister.
	SinkRemoved func(sink string)
	// Status fires on the first real failure of a destination and when that
	// destination later becomes connected. WaitingForConnection is not a
	// failure. A repeated failure, including a different error string, does
	// not fire again until the destination has recovered.
	Status func(sink, status, lastError string)
}

// Dispatcher is the single delivery loop for one process.
// Register a sink only when that sink is configured. Kafka is registered
// by the API process when STORAGE_KAFKA_BROKERS is non-empty.
//
// Per object key, a sink does not receive a later row while an earlier row
// for the same tenant, bucket, and key still has that sink pending. One
// stuck key does not block other keys or other sinks. An empty key is not
// ordered. When the earlier attempt is dead or delivered, later rows proceed.
type Dispatcher struct {
	store      OutboxStore
	cfg        Config
	leader     LeaderFunc
	obs        Observer
	purgeEvery time.Duration

	mu        sync.Mutex
	sinks     []EventSink
	byName    map[string]EventSink
	reported  map[string]bool // sink name -> a real failure is open
	lastPurge time.Time
	cancel    context.CancelFunc
	closed    bool
	fenced    bool
	onFence   func()
}

// NewDispatcher builds a dispatcher. store may be nil only in tests that never tick.
func NewDispatcher(store OutboxStore, sinks []EventSink, cfg Config, leader LeaderFunc, obs Observer) *Dispatcher {
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = DefaultMaxAttempts
	}
	if cfg.Retention <= 0 {
		cfg.Retention = DefaultRetention
	}
	if cfg.DeadRetention <= 0 {
		cfg.DeadRetention = DefaultDeadRetention
	}
	d := &Dispatcher{
		store:      store,
		cfg:        cfg,
		leader:     leader,
		obs:        obs,
		purgeEvery: PurgeEvery,
		byName:     map[string]EventSink{},
	}
	for _, sk := range sinks {
		d.registerLocked(sk)
	}
	return d
}

// SetOnFence registers the callback that drops this process's leader lease.
// The API process passes ha.Leader.Resign. Nil leaves the lease to renew.
func (d *Dispatcher) SetOnFence(fn func()) {
	if d == nil {
		return
	}
	d.mu.Lock()
	d.onFence = fn
	d.mu.Unlock()
}

func (d *Dispatcher) fencedAlready() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.fenced
}

// noteFenced stops this dispatcher and drops the lease once.
func (d *Dispatcher) noteFenced(cause error) {
	d.mu.Lock()
	if d.fenced {
		d.mu.Unlock()
		return
	}
	d.fenced = true
	fn := d.onFence
	d.mu.Unlock()
	slog.Warn("event dispatcher stopped", "err", cause)
	if fn != nil {
		fn()
	}
}

// Register adds a sink. Safe to call before Run. Replacing a name closes the previous sink.
func (d *Dispatcher) Register(sink EventSink) {
	if sink == nil || sink.Name() == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.registerLocked(sink)
}

func (d *Dispatcher) registerLocked(sink EventSink) {
	if sink == nil || sink.Name() == "" {
		return
	}
	if _, ok := d.byName[sink.Name()]; ok {
		if prev := d.byName[sink.Name()]; prev != sink {
			_ = prev.Close()
		}
		for i, existing := range d.sinks {
			if existing.Name() == sink.Name() {
				d.sinks[i] = sink
				d.byName[sink.Name()] = sink
				return
			}
		}
	}
	d.sinks = append(d.sinks, sink)
	d.byName[sink.Name()] = sink
}

// Unregister removes a sink and closes it. Pending rows that named it
// keep that name (copied at insert) and fail until dead or a later retry.
func (d *Dispatcher) Unregister(name string) {
	if d == nil || name == "" {
		return
	}
	d.mu.Lock()
	prev, ok := d.byName[name]
	if !ok {
		d.mu.Unlock()
		return
	}
	delete(d.byName, name)
	delete(d.reported, name)
	next := make([]EventSink, 0, len(d.sinks))
	for _, sk := range d.sinks {
		if sk.Name() == name {
			continue
		}
		next = append(next, sk)
	}
	d.sinks = next
	if prev != nil {
		_ = prev.Close()
	}
	gone := d.obs.SinkRemoved
	d.mu.Unlock()
	if gone != nil {
		gone(name)
	}
}

// HasSink reports whether name is currently registered.
func (d *Dispatcher) HasSink(name string) bool {
	if d == nil || name == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.byName[name]
	return ok
}

// HealthReporter is optional. Sinks that track a connection or the last
// publish error implement it. Status is "connected" or "degraded".
type HealthReporter interface {
	Health(ctx context.Context) (status, lastError string)
}

// Health reports one registered sink. registered is false when name is not
// in the dispatcher; status is then "disabled".
func (d *Dispatcher) Health(ctx context.Context, name string) (registered bool, status, lastError string) {
	if d == nil || name == "" {
		return false, "disabled", ""
	}
	d.mu.Lock()
	sk := d.byName[name]
	d.mu.Unlock()
	if sk == nil {
		return false, "disabled", ""
	}
	if h, ok := sk.(HealthReporter); ok {
		st, errStr := h.Health(ctx)
		if st == "" {
			st = "connected"
		}
		return true, st, errStr
	}
	return true, "connected", ""
}

// SinkNames returns the snapshot of registered sink names.
func (d *Dispatcher) SinkNames() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]string, 0, len(d.sinks))
	for _, sk := range d.sinks {
		out = append(out, sk.Name())
	}
	return out
}

// Run polls until ctx is cancelled.
func (d *Dispatcher) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	defer cancel()
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	if errors.Is(d.ProcessOnce(ctx, time.Now().UTC()), ErrStoreFenced) {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if errors.Is(d.ProcessOnce(ctx, time.Now().UTC()), ErrStoreFenced) {
				return
			}
		}
	}
}

// Close stops the loop and closes sinks.
func (d *Dispatcher) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	if d.cancel != nil {
		d.cancel()
	}
	var err error
	for _, sk := range d.sinks {
		err = errors.Join(err, sk.Close())
	}
	d.sinks = nil
	d.byName = map[string]EventSink{}
	return err
}

// ProcessOnce delivers due rows and, on the purge interval, deletes expired ones.
// now is injectable so tests can step past backoff without sleeping.
// A non-leader updates the pending gauge and does not deliver or purge.
func (d *Dispatcher) ProcessOnce(ctx context.Context, now time.Time) error {
	if d == nil || d.store == nil {
		return nil
	}
	if d.fencedAlready() {
		return ErrStoreFenced
	}
	now = now.UTC()
	if n, err := d.store.CountOutbox(ctx, StatusPending); err == nil && d.obs.Pending != nil {
		d.obs.Pending(n)
	}
	if sinks := d.sinkSnapshot(); sinks != nil {
		d.observeSinkHealth(ctx, sinks)
	}
	if d.leader != nil && !d.leader(ctx) {
		return nil
	}
	sinks, purge := d.snapshot(now)
	if sinks == nil {
		return nil
	}
	var errs error
	if purge {
		if err := d.purge(ctx, now); err != nil {
			if StoreFenced(err) {
				d.noteFenced(err)
				return ErrStoreFenced
			}
			errs = errors.Join(errs, err)
		}
	}
	due, err := d.store.ListDueOutbox(ctx, now, 64)
	if err != nil {
		if StoreFenced(err) {
			d.noteFenced(err)
			return ErrStoreFenced
		}
		return errors.Join(errs, err)
	}
	for i := range due {
		if ctx.Err() != nil {
			return errors.Join(errs, ctx.Err())
		}
		if d.leader != nil && !d.leader(ctx) {
			d.noteFenced(ErrStoreFenced)
			return ErrStoreFenced
		}
		if err := d.deliver(ctx, sinks, &due[i], now); err != nil {
			if StoreFenced(err) {
				d.noteFenced(err)
				return ErrStoreFenced
			}
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// sinkSnapshot copies the registered sinks. Nil means the dispatcher is closed.
func (d *Dispatcher) sinkSnapshot() map[string]EventSink {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	sinks := make(map[string]EventSink, len(d.byName))
	for name, sk := range d.byName {
		sinks[name] = sk
	}
	return sinks
}

// observeSinkHealth publishes the degraded gauge and the first-failure /
// recovery signal. It does not treat WaitingForConnection as a failure.
func (d *Dispatcher) observeSinkHealth(ctx context.Context, sinks map[string]EventSink) {
	type change struct {
		sink, status, last string
	}
	var changes []change
	for name, sk := range sinks {
		status, last := "connected", ""
		if h, ok := sk.(HealthReporter); ok {
			status, last = h.Health(ctx)
			if status == "" {
				status = "connected"
			}
		}
		degraded := status == "degraded"
		real := degraded && last != "" && last != WaitingForConnection
		d.mu.Lock()
		if _, still := d.byName[name]; !still {
			d.mu.Unlock()
			continue
		}
		if d.reported == nil {
			d.reported = map[string]bool{}
		}
		prev := d.reported[name]
		switch {
		case real && !prev:
			d.reported[name] = true
			changes = append(changes, change{name, "degraded", last})
		case !degraded && prev:
			d.reported[name] = false
			changes = append(changes, change{name, "connected", ""})
		}
		if d.obs.Degraded != nil {
			d.obs.Degraded(name, degraded)
		}
		d.mu.Unlock()
	}
	if d.obs.Status == nil {
		return
	}
	for _, c := range changes {
		d.obs.Status(c.sink, c.status, c.last)
	}
}

// snapshot copies the sink map and decides whether this tick should purge.
// A nil map means the dispatcher is closed. The lock is not held across Publish.
func (d *Dispatcher) snapshot(now time.Time) (map[string]EventSink, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, false
	}
	purge := d.purgeEvery == 0 || d.lastPurge.IsZero() || now.Sub(d.lastPurge) >= d.purgeEvery
	if purge {
		d.lastPurge = now
	}
	sinks := make(map[string]EventSink, len(d.byName))
	for name, sk := range d.byName {
		sinks[name] = sk
	}
	return sinks, purge
}

func (d *Dispatcher) purge(ctx context.Context, now time.Time) error {
	var errs error
	if _, err := d.store.PurgeOutbox(ctx, StatusDelivered, now.Add(-d.cfg.Retention)); err != nil {
		errs = errors.Join(errs, err)
	}
	deadBefore := now.Add(-d.cfg.DeadRetention)
	if _, err := d.store.PurgeOutbox(ctx, StatusDead, deadBefore); err != nil {
		errs = errors.Join(errs, err)
	}
	if _, err := d.store.PurgeOutbox(ctx, StatusDropped, deadBefore); err != nil {
		errs = errors.Join(errs, err)
	}
	return errs
}

func (d *Dispatcher) deliver(ctx context.Context, sinks map[string]EventSink, rec *OutboxRecord, now time.Time) error {
	if rec.Status != StatusPending {
		return nil
	}
	dirty := false
	for name, att := range rec.Attempts {
		if !SinkStillPending(att) {
			continue
		}
		if att.NextAttemptAt.After(now) {
			continue
		}
		if rec.Key != "" {
			blocked, err := d.store.EarlierPending(ctx, rec.TenantID, rec.Bucket, rec.Key, name, rec.ID, rec.CreatedAt)
			if err != nil {
				return err
			}
			if blocked {
				// Leave attempts and NextAttemptAt alone so a blocked row is
				// not rewritten on every tick. Other keys still deliver.
				continue
			}
		}
		if d.fencedAlready() || (d.leader != nil && !d.leader(ctx)) {
			return ErrStoreFenced
		}
		dirty = true
		sink := sinks[name]
		var pubErr error
		if sink == nil {
			pubErr = errors.New("sink not registered")
		} else {
			pubErr = sink.Publish(ctx, rec.Event())
		}
		if pubErr != nil && ctx.Err() != nil && errors.Is(pubErr, context.Canceled) {
			// Shutdown is not a failed attempt. Leave the sink pending.
			continue
		}
		if pubErr == nil {
			att.Status = StatusDelivered
			att.LastError = ""
			rec.Attempts[name] = att
			if d.obs.Delivered != nil {
				d.obs.Delivered(name)
			}
			if d.obs.Lag != nil && !rec.CreatedAt.IsZero() {
				lag := now.Sub(rec.CreatedAt).Seconds()
				if lag < 0 {
					lag = 0
				}
				d.obs.Lag(name, lag)
			}
			continue
		}
		var terminal *TerminalError
		if errors.As(pubErr, &terminal) {
			att.Attempts++
			att.LastError = pubErr.Error()
			att.Status = StatusDead
			rec.Attempts[name] = att
			if d.obs.Failed != nil {
				d.obs.Failed(name, "terminal")
			}
			if d.obs.Dead != nil {
				d.obs.Dead(name)
			}
			slog.Warn("event sink rejected",
				"event_id", rec.ID, "sink", name, "attempts", att.Attempts, "err", att.LastError)
			continue
		}
		att.Attempts++
		att.LastError = pubErr.Error()
		if d.obs.Failed != nil {
			d.obs.Failed(name, "publish")
		}
		if att.Attempts >= d.cfg.MaxAttempts {
			att.Status = StatusDead
			rec.Attempts[name] = att
			if d.obs.Dead != nil {
				d.obs.Dead(name)
			}
			slog.Warn("event sink exhausted attempts",
				"event_id", rec.ID, "sink", name, "attempts", att.Attempts, "err", att.LastError)
			continue
		}
		delay := BackoffDuration(att.Attempts)
		var retryAfter *RetryAfterError
		if errors.As(pubErr, &retryAfter) && retryAfter.After > delay {
			delay = retryAfter.After
		}
		if delay > maxBackoff {
			delay = maxBackoff
		}
		att.Status = StatusPending
		att.NextAttemptAt = now.Add(delay)
		rec.Attempts[name] = att
		slog.Warn("event sink delivery failed",
			"event_id", rec.ID, "sink", name, "attempts", att.Attempts, "next_attempt_at", att.NextAttemptAt, "err", att.LastError)
	}
	if !dirty {
		return nil
	}
	AggregateStatus(rec, now)
	return d.store.SaveOutbox(ctx, *rec)
}

// BackoffDuration is exponential from 1s, capped at 5m, with ±20% jitter.
// attempt is the count after a failure (1 = first failure).
func BackoffDuration(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := attempt - 1
	if shift > 16 {
		shift = 16
	}
	d := baseBackoff << shift
	if d > maxBackoff || d <= 0 {
		d = maxBackoff
	}
	span := float64(d) * 0.2
	delta := (rand.Float64()*2 - 1) * span
	out := time.Duration(float64(d) + delta)
	if out < 100*time.Millisecond {
		out = 100 * time.Millisecond
	}
	if out > maxBackoff {
		out = maxBackoff
	}
	return out
}
