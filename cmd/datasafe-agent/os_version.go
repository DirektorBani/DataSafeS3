package main

import "fmt"

// formatWindowsVersion names the product from the build number. Windows 11
// still reports 10.0 from RtlGetVersion; build 22000 and later is Windows 11.
func formatWindowsVersion(major, minor, build uint32, arch string) string {
	product := fmt.Sprintf("Windows %d.%d", major, minor)
	if major == 10 && minor == 0 {
		if build >= 22000 {
			product = "Windows 11"
		} else {
			product = "Windows 10"
		}
	}
	text := fmt.Sprintf("%s (%d.%d.%d)", product, major, minor, build)
	if arch != "" {
		text += " " + arch
	}
	return text
}
