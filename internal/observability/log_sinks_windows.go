//go:build windows

package observability

import "github.com/DirektorBani/datasafe/internal/metadata"

func buildPlatformLogSinks(cfg metadata.LoggingConfig) []LogSink {
	return nil
}
