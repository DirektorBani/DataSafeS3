//go:build !linux

package observability

type HostStats struct {
	DiskTotalBytes uint64
	DiskFreeBytes  uint64
	DiskUsedBytes  uint64
	DiskUsedPct    float64
	CPULoad1       float64
	MemTotalBytes  uint64
	MemFreeBytes   uint64
	MemUsedPct     float64
	NetInBytes     uint64
	NetOutBytes    uint64
}

func CollectHostStats(dataDir string) HostStats {
	return HostStats{}
}
