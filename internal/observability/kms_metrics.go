package observability

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// KMS metrics are registered on first use. An idle node that never enables
// Vault does not allocate them.
var (
	kmsRegOnce sync.Once

	kmsRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_kms_requests_total",
		Help: "Vault and local KMS calls by operation, provider, and result.",
	}, []string{"op", "provider", "result"})
	kmsRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "datasafe_kms_request_duration_seconds",
		Help:    "KMS call latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"op", "provider"})
	kmsAvailable = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_kms_available",
		Help: "1 when this node can reach Vault Transit. Published only while Vault is enabled.",
	})
	kmsCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "datasafe_kms_dek_cache_hits_total",
		Help: "Unwrapped DEKs served from the node cache.",
	})
	kmsCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "datasafe_kms_dek_cache_misses_total",
		Help: "DEK unwraps that missed the node cache.",
	})
	kmsJobs = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_kms_jobs_objects_total",
		Help: "Objects processed by KMS background jobs.",
	}, []string{"job", "result"})
)

func ensureKMSMetrics() {
	kmsRegOnce.Do(func() {
		prometheus.MustRegister(kmsRequestsTotal, kmsRequestDuration, kmsAvailable, kmsCacheHits, kmsCacheMisses)
	})
}

var jobRegOnce sync.Once

// ObserveKMSJob counts one object finished by a leader job.
// Registration is separate from Vault metrics, so an idle node that never
// runs a job does not publish datasafe_kms_*.
func ObserveKMSJob(job, result string) {
	jobRegOnce.Do(func() {
		prometheus.MustRegister(kmsJobs)
	})
	kmsJobs.WithLabelValues(job, result).Inc()
}

// ResetKMSJobMetricsForTest removes the job counter from the default registry.
// Package tests share that registry, and an idle /metrics check must not see it.
func ResetKMSJobMetricsForTest() {
	prometheus.Unregister(kmsJobs)
	jobRegOnce = sync.Once{}
}

// ObserveKMS records one KMS call. The first call registers the KMS metrics.
func ObserveKMS(op, provider, result string, d time.Duration) {
	ensureKMSMetrics()
	kmsRequestsTotal.WithLabelValues(op, provider, result).Inc()
	kmsRequestDuration.WithLabelValues(op, provider).Observe(d.Seconds())
}

// SetKMSAvailable sets this node's Vault gauge. It is not called when Vault is unused.
func SetKMSAvailable(up bool) {
	ensureKMSMetrics()
	if up {
		kmsAvailable.Set(1)
		return
	}
	kmsAvailable.Set(0)
}

// KMSCacheHit counts a DEK cache hit.
func KMSCacheHit() {
	ensureKMSMetrics()
	kmsCacheHits.Inc()
}

// KMSCacheMiss counts a DEK cache miss.
func KMSCacheMiss() {
	ensureKMSMetrics()
	kmsCacheMisses.Inc()
}
