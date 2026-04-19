// Package version exposes build metadata populated at link time via -ldflags.
package version

var (
	Commit    = "dev"
	BuildDate = "unknown"
)
