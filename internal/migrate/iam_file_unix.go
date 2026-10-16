//go:build !windows

package migrate

import "os"

func restrictKeysFile(path string) error {
	return os.Chmod(path, 0o600)
}
