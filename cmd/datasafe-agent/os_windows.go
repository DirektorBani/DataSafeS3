//go:build windows

package main

import (
	"runtime"

	"golang.org/x/sys/windows"
)

func osDescription() string {
	v := windows.RtlGetVersion()
	return formatWindowsVersion(v.MajorVersion, v.MinorVersion, v.BuildNumber, runtime.GOARCH)
}
