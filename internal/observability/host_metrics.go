package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	hostDiskTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_disk_total_bytes",
		Help: "Total disk bytes on data volume.",
	})
	hostDiskFree = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_disk_free_bytes",
		Help: "Free disk bytes on data volume.",
	})
	hostDiskUsed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_disk_used_bytes",
		Help: "Used disk bytes on data volume.",
	})
	hostDiskUsedPct = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_disk_used_percent",
		Help: "Disk used percent on data volume.",
	})
	hostCPULoad = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_cpu_load1",
		Help: "1-minute CPU load average.",
	})
	hostMemTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_memory_total_bytes",
		Help: "Total system memory.",
	})
	hostMemFree = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_memory_free_bytes",
		Help: "Free system memory.",
	})
	hostMemUsedPct = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_memory_used_percent",
		Help: "Memory used percent.",
	})
	hostNetIn = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_network_in_bytes_total",
		Help: "Network bytes received (non-loopback).",
	})
	hostNetOut = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "datasafe_host_network_out_bytes_total",
		Help: "Network bytes transmitted (non-loopback).",
	})
)

var hostDataDir string

func SetHostDataDir(dir string) {
	hostDataDir = dir
}

func refreshHostMetrics() {
	s := CollectHostStats(hostDataDir)
	hostDiskTotal.Set(float64(s.DiskTotalBytes))
	hostDiskFree.Set(float64(s.DiskFreeBytes))
	hostDiskUsed.Set(float64(s.DiskUsedBytes))
	hostDiskUsedPct.Set(s.DiskUsedPct)
	hostCPULoad.Set(s.CPULoad1)
	hostMemTotal.Set(float64(s.MemTotalBytes))
	hostMemFree.Set(float64(s.MemFreeBytes))
	hostMemUsedPct.Set(s.MemUsedPct)
	hostNetIn.Set(float64(s.NetInBytes))
	hostNetOut.Set(float64(s.NetOutBytes))
}
