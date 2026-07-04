package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	erasureDegradedShardSets = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_erasure_degraded_shard_sets",
		Help: "Erasure-coded object sets with missing shards within parity tolerance.",
	})
	erasureHealBytesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "datasafe_erasure_heal_bytes_total",
		Help: "Total bytes rewritten by erasure self-heal.",
	})
)

func SetErasureDegradedSets(n int) {
	erasureDegradedShardSets.Set(float64(n))
}

func AddErasureHealBytes(n int64) {
	if n > 0 {
		erasureHealBytesTotal.Add(float64(n))
	}
}
