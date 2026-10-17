//go:build linux

package main

import (
	"os"
	"runtime"
	"strings"
)

func osDescription() string {
	pretty := "linux"
	raw, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				pretty = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
				break
			}
		}
	}
	if pretty == "" {
		pretty = "linux"
	}
	return pretty + " " + runtime.GOARCH
}
