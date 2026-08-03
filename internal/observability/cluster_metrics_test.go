package observability

import "testing"

func TestSetClusterNodeStatuses(t *testing.T) {
	SetClusterNodeStatuses("degraded", []ClusterNodeSample{
		{ID: "n0", Address: "10.0.0.1:9000", Role: "primary", Status: "healthy"},
		{ID: "n1", Address: "10.0.0.2:9000", Role: "replica", Status: "offline"},
	})
	SetHALeaderMetrics(true, true)
	// Smoke: no panic; aggregate helpers remain callable.
	SetClusterMetrics(2, 1, 1)
	SetHALeaderMetrics(false, false)
}
