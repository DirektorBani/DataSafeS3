//go:build unix

package main

import (
	"os"
	"syscall"
)

func fileID(info os.FileInfo) (uint64, uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(st.Dev), st.Ino, true
}

func isReparse(string, os.FileInfo) bool { return false }
