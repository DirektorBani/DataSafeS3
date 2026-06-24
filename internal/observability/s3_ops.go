package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	s3ReadOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_s3_read_ops_total",
		Help: "Total S3 read operations (GetObject, HeadObject) per bucket.",
	}, []string{"bucket"})
	s3WriteOps = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "datasafe_s3_write_ops_total",
		Help: "Total S3 write operations (PutObject, DeleteObject, multipart) per bucket.",
	}, []string{"bucket"})
)

func IncS3ReadOps(bucket string)  { s3ReadOps.WithLabelValues(bucket).Inc() }
func IncS3WriteOps(bucket string) { s3WriteOps.WithLabelValues(bucket).Inc() }
