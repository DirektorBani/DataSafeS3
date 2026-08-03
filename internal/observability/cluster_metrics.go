package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ClusterNodeSample is one probed cluster member for Prometheus.
type ClusterNodeSample struct {
	ID      string
	Address string
	Role    string
	Status  string // healthy | degraded | offline
}

var (
	clusterNodeUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasafe_cluster_node_up",
		Help: "1 if cluster node /healthz is healthy, 0 otherwise.",
	}, []string{"node_id", "address", "role"})

	clusterNodeStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasafe_cluster_node_status",
		Help: "Node status code: 2=healthy, 1=degraded, 0=offline.",
	}, []string{"node_id", "address", "role", "status"})

	clusterOverallStatus = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_cluster_overall_status",
		Help: "Cluster overall: 2=healthy, 1=degraded, 0=offline.",
	})

	haEnabled = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_ha_enabled",
		Help: "1 if storage HA leader election is enabled.",
	})

	haIsLeader = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_ha_is_leader",
		Help: "1 if this process holds the storage HA lock.",
	})
)

func statusCode(status string) float64 {
	switch status {
	case "healthy":
		return 2
	case "degraded":
		return 1
	default:
		return 0
	}
}

// SetClusterNodeStatuses replaces per-node gauges and aggregate totals.
func SetClusterNodeStatuses(overall string, nodes []ClusterNodeSample) {
	clusterNodeUp.Reset()
	clusterNodeStatus.Reset()
	healthy, offline := 0, 0
	for _, n := range nodes {
		id := n.ID
		if id == "" {
			id = n.Address
		}
		role := n.Role
		if role == "" {
			role = "storage"
		}
		addr := n.Address
		up := 0.0
		if n.Status == "healthy" {
			up = 1
			healthy++
		} else if n.Status == "offline" {
			offline++
		}
		clusterNodeUp.WithLabelValues(id, addr, role).Set(up)
		clusterNodeStatus.WithLabelValues(id, addr, role, n.Status).Set(statusCode(n.Status))
	}
	SetClusterMetrics(len(nodes), healthy, offline)
	clusterOverallStatus.Set(statusCode(overall))
}

// SetHALeaderMetrics publishes HA election state for this process.
func SetHALeaderMetrics(enabled, isLeader bool) {
	if enabled {
		haEnabled.Set(1)
	} else {
		haEnabled.Set(0)
	}
	if isLeader {
		haIsLeader.Set(1)
	} else {
		haIsLeader.Set(0)
	}
}
