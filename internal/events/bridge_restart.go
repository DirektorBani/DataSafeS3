package events

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// The restart counter lives in this package. observability imports metadata,
// and metadata imports events, so the bridge supervisor cannot import observability.
var eventsBridgeRestarts = promauto.NewCounter(prometheus.CounterOpts{
	Name: "datasafe_events_bridge_restarts_total",
	Help: "Restarts of the event delivery bridge child after it exited. The child is the Kafka producer process.",
})

func noteBridgeRestart() {
	eventsBridgeRestarts.Inc()
}
