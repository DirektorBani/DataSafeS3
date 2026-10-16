//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func fileID(info os.FileInfo) (uint64, uint64, bool) {
	if info == nil {
		return 0, 0, false
	}
	return 0, 0, false
}

func isReparse(path string, info os.FileInfo) bool {
	if info != nil && info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attr, err := windows.GetFileAttributes(p)
	if err != nil {
		return false
	}
	return attr&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
