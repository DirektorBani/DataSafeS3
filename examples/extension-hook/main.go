// Sample extension hook: reject uploads over 50 MiB.
// Build: go build -tags=extensions -o extension-hook .
// Register at init by importing this package from a custom storage-server main (fork) or use webhook validation instead.
package main

import (
	"context"
	"fmt"

	"github.com/DirektorBani/datasafe/internal/extensions"
)

func init() {
	extensions.RegisterObjectUploadValidator(func(ctx context.Context, in extensions.UploadValidationContext) error {
		const maxBytes = 50 * 1024 * 1024
		if in.Size > maxBytes {
			return fmt.Errorf("extension-hook: object too large (%d > %d)", in.Size, maxBytes)
		}
		return nil
	})
}

func main() {
	// This binary documents the hook registration pattern; wire into storage-server via -tags=extensions build.
}
