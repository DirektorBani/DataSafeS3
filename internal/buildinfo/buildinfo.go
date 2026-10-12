// Package buildinfo holds the release version and git commit.
// The storage-server image and release.yml set them with -ldflags -X.
// A local go test or go run keeps the defaults.
package buildinfo

var (
	Version = "dev"
	Commit  = "unknown"
)
