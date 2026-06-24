package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	clusterNodesTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_cluster_nodes_total",
		Help: "Configured cluster nodes.",
	})
	clusterNodesHealthy = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_cluster_nodes_healthy",
		Help: "Cluster nodes responding healthy to /healthz.",
	})
	clusterNodesOffline = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_cluster_nodes_offline",
		Help: "Cluster nodes offline or unreachable.",
	})
	objectsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_objects_total",
		Help: "Total latest objects across all buckets.",
	})
	versionsTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_versions_total",
		Help: "Total object versions stored.",
	})
	replicationQueue = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_replication_queue",
		Help: "Pending gateway replication tasks.",
	})
	webhookFailures = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_webhook_failures",
		Help: "Recent failed webhook deliveries.",
	})
	multipartActive = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_multipart_uploads_active",
		Help: "Active multipart uploads.",
	})
	storagePerBucket = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasafe_storage_per_bucket",
		Help: "Stored bytes per bucket.",
	}, []string{"bucket"})
	bucketObjects = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasafe_bucket_objects",
		Help: "Latest object count per bucket.",
	}, []string{"bucket"})
	storagePerTenant = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "datasafe_storage_per_tenant",
		Help: "Stored bytes per tenant.",
	}, []string{"tenant"})
	ldapSyncTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_ldap_sync_total",
		Help: "LDAP scheduled sync runs by result.",
	}, []string{"result"})
)

// ExtendedStats holds optional extended Prometheus gauge inputs.
type ExtendedStats struct {
	ObjectsTotal     int
	VersionsTotal    int
	ReplicationQueue int
	WebhookFailures  int
	MultipartActive  int
	ObjectsPerBucket map[string]int
	StoragePerBucket map[string]int64
	StoragePerTenant map[string]int64
}

type ExtendedStatsFn func() ExtendedStats

var extendedFn ExtendedStatsFn

func SetExtendedStats(fn ExtendedStatsFn) {
	extendedFn = fn
}

func refreshExtended() {
	if extendedFn == nil {
		return
	}
	s := extendedFn()
	objectsTotal.Set(float64(s.ObjectsTotal))
	versionsTotal.Set(float64(s.VersionsTotal))
	replicationQueue.Set(float64(s.ReplicationQueue))
	webhookFailures.Set(float64(s.WebhookFailures))
	multipartActive.Set(float64(s.MultipartActive))
	bucketObjects.Reset()
	for b, n := range s.ObjectsPerBucket {
		bucketObjects.WithLabelValues(b).Set(float64(n))
	}
	storagePerBucket.Reset()
	for b, bytes := range s.StoragePerBucket {
		storagePerBucket.WithLabelValues(b).Set(float64(bytes))
	}
	storagePerTenant.Reset()
	for t, bytes := range s.StoragePerTenant {
		storagePerTenant.WithLabelValues(t).Set(float64(bytes))
	}
}

func SetClusterMetrics(total, healthy, offline int) {
	clusterNodesTotal.Set(float64(total))
	clusterNodesHealthy.Set(float64(healthy))
	clusterNodesOffline.Set(float64(offline))
}

func IncLDAPSync(result string) {
	ldapSyncTotal.WithLabelValues(result).Inc()
}

var postgresReplicationLag = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "datasafe_postgres_replication_lag_seconds",
	Help: "PostgreSQL replication lag in seconds when monitoring is available.",
})

func SetPostgresReplicationLag(seconds float64) {
	postgresReplicationLag.Set(seconds)
}
