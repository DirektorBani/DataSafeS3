//go:build windows

package main

import (
	"fmt"
	"runtime"

	"golang.org/x/sys/windows"
)

func osDescription() string {
	v := windows.RtlGetVersion()
	return fmt.Sprintf("windows %d.%d.%d %s", v.MajorVersion, v.MinorVersion, v.BuildNumber, runtime.GOARCH)
}
