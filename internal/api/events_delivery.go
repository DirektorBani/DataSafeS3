package api

import (
	"net/http"
	"time"

	"github.com/DirektorBani/datasafe/internal/events"
)

// knownDeliverySinks are always listed so the console page is stable when a
// sink is disabled. Registered sinks that are not in this list are appended.
var knownDeliverySinks = []string{"kafka", "nats", "webhook"}

func (s *Server) handleEventDelivery(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var total int
	bySink := map[string]int{}
	if st := s.outboxStore(); st != nil {
		if n, err := st.CountOutbox(ctx, events.StatusPending); err == nil {
			total = n
		}
		if m, err := st.PendingBySink(ctx); err == nil && m != nil {
			bySink = m
		}
	}
	names := append([]string{}, knownDeliverySinks...)
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
	}
	if s.eventDisp != nil {
		for _, n := range s.eventDisp.SinkNames() {
			if !seen[n] {
				names = append(names, n)
				seen[n] = true
			}
		}
	}
	sinks := make([]map[string]any, 0, len(names))
	for _, name := range names {
		status, last := "disabled", ""
		if s.eventDisp != nil {
			if reg, st, errStr := s.eventDisp.Health(ctx, name); reg {
				status, last = st, errStr
			}
		}
		sinks = append(sinks, map[string]any{
			"name":        name,
			"status":      status,
			"last_error":  last,
			"queue_depth": bySink[name],
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"queue_depth": total,
		"sinks":       sinks,
	})
}

func (s *Server) handleEventTest(w http.ResponseWriter, r *http.Request) {
	if s.eventDisp == nil || s.outboxStore() == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "event delivery is not configured"})
		return
	}
	now := time.Now().UTC()
	attempts := map[string]events.SinkAttempt{}
	for _, name := range s.eventDisp.SinkNames() {
		if name == bucketWebhookSinkName {
			continue
		}
		attempts[name] = events.SinkAttempt{Status: events.StatusPending, NextAttemptAt: now}
	}
	if len(attempts) == 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "no event sinks configured"})
		return
	}
	rec := events.NormalizeOutbox(events.OutboxRecord{
		EventType: "TestEvent",
		Bucket:    "_datasafe",
		Key:       "test",
		Payload:   []byte(`{"bucket":"_datasafe","key":"test","size":0}`),
		CreatedAt: now,
		Attempts:  attempts,
	}, now)
	if _, err := s.outboxStore().InsertOutbox(r.Context(), rec); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to queue test event"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"event_id": rec.ID})
}
