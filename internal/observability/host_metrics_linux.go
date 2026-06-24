//go:build linux

package observability

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

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
	var s HostStats
	if dataDir == "" {
		dataDir = "."
	}
	path := dataDir
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		path = filepath.Dir(path)
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err == nil {
		s.DiskTotalBytes = stat.Blocks * uint64(stat.Bsize)
		s.DiskFreeBytes = stat.Bavail * uint64(stat.Bsize)
		if s.DiskTotalBytes > 0 {
			s.DiskUsedBytes = s.DiskTotalBytes - s.DiskFreeBytes
			s.DiskUsedPct = float64(s.DiskUsedBytes) / float64(s.DiskTotalBytes) * 100
		}
	}
	if f, err := os.Open("/proc/loadavg"); err == nil {
		sc := bufio.NewScanner(f)
		if sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) > 0 {
				s.CPULoad1, _ = strconv.ParseFloat(fields[0], 64)
			}
		}
		_ = f.Close()
	}
	if f, err := os.Open("/proc/meminfo"); err == nil {
		vals := map[string]uint64{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			parts := strings.Fields(sc.Text())
			if len(parts) < 2 {
				continue
			}
			key := strings.TrimSuffix(parts[0], ":")
			v, _ := strconv.ParseUint(parts[1], 10, 64)
			vals[key] = v * 1024
		}
		_ = f.Close()
		s.MemTotalBytes = vals["MemTotal"]
		free := vals["MemFree"] + vals["Buffers"] + vals["Cached"]
		s.MemFreeBytes = free
		if s.MemTotalBytes > 0 {
			used := s.MemTotalBytes - free
			s.MemUsedPct = float64(used) / float64(s.MemTotalBytes) * 100
		}
	}
	if f, err := os.Open("/proc/net/dev"); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.Contains(line, ":") || strings.HasPrefix(line, "Inter-") {
				continue
			}
			parts := strings.Fields(strings.Replace(line, ":", " ", 1))
			if len(parts) < 10 {
				continue
			}
			if parts[0] == "lo" {
				continue
			}
			in, _ := strconv.ParseUint(parts[1], 10, 64)
			out, _ := strconv.ParseUint(parts[9], 10, 64)
			s.NetInBytes += in
			s.NetOutBytes += out
		}
		_ = f.Close()
	}
	return s
}
