package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	siteReplicationLag = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_site_replication_lag_seconds",
		Help: "Seconds since last successful site replication task.",
	})
	siteReplicationQueue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_site_replication_queue_depth",
		Help: "Pending site replication tasks.",
	})
)

func SetSiteReplicationMetrics(pending int, lagSeconds float64) {
	siteReplicationQueue.Set(float64(pending))
	siteReplicationLag.Set(lagSeconds)
}
