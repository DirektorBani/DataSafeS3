package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Event delivery metrics. The dispatcher updates these.
// The overview dashboard row "Event delivery" and deploy/docker/prometheus/alerts.yml read them.
var (
	eventsOutboxPending = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_events_outbox_pending",
		Help: "Notification outbox rows waiting for delivery.",
	})
	eventsDeliveredTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_events_delivered_total",
		Help: "Notification deliveries that succeeded, by sink.",
	}, []string{"sink"})
	eventsFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_events_failed_total",
		Help: "Notification delivery attempts that failed, by sink and reason.",
	}, []string{"sink", "reason"})
	eventsDeadTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_events_dead_total",
		Help: "Notification sinks that exhausted retries, by sink.",
	}, []string{"sink"})
	eventsDeliveryLag = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "datasafe_events_delivery_lag_seconds",
		Help:    "Seconds from outbox insert to a successful sink delivery.",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300, 900, 3600},
	}, []string{"sink"})
	eventsDroppedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_events_dropped_total",
		Help: "Outbox rows marked dropped so object writes are not blocked, by reason.",
	}, []string{"reason"})
	eventsSinkDegraded = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasafe_events_sink_degraded",
		Help: "1 while a delivery destination is degraded, including before the first successful connection, else 0.",
	}, []string{"sink"})
)

func SetEventsOutboxPending(n int) {
	eventsOutboxPending.Set(float64(n))
}

func AddEventsDelivered(sink string) {
	eventsDeliveredTotal.WithLabelValues(sink).Inc()
}

func AddEventsFailed(sink, reason string) {
	eventsFailedTotal.WithLabelValues(sink, reason).Inc()
}

func AddEventsDead(sink string) {
	eventsDeadTotal.WithLabelValues(sink).Inc()
}

func ObserveEventsDeliveryLag(sink string, seconds float64) {
	eventsDeliveryLag.WithLabelValues(sink).Observe(seconds)
}

func AddEventsDropped(n int, reason string) {
	if n <= 0 {
		return
	}
	eventsDroppedTotal.WithLabelValues(reason).Add(float64(n))
}

// SetEventsSinkDegraded sets the 0/1 gauge for one destination.
func SetEventsSinkDegraded(sink string, degraded bool) {
	if sink == "" {
		return
	}
	v := 0.0
	if degraded {
		v = 1
	}
	eventsSinkDegraded.WithLabelValues(sink).Set(v)
}

// ClearEventsSinkDegraded removes the series so an unregistered destination does not stay alerting.
func ClearEventsSinkDegraded(sink string) {
	if sink == "" {
		return
	}
	eventsSinkDegraded.DeleteLabelValues(sink)
}
