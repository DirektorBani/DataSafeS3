//go:build !windows

package main

// purgeDesktopSecrets is a no-op off Windows. Linux uninstall does not
// delete Secret Service entries.
func purgeDesktopSecrets() (int, error) {
	return 0, nil
}
