//go:build !linux && !windows

package main

import "runtime"

func osDescription() string {
	return runtime.GOOS + " " + runtime.GOARCH
}
