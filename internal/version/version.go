// Package version exposes build metadata injected at link time.
package version

import "fmt"

// These values are overridden with:
//
//	-ldflags "-X github.com/asimmons91/pico-editor/internal/version.Version=... \
//	          -X github.com/asimmons91/pico-editor/internal/version.Commit=... \
//	          -X github.com/asimmons91/pico-editor/internal/version.BuildDate=..."
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// String returns the user-visible version line.
func String() string {
	return format(Version, Commit, BuildDate)
}

func format(buildVersion, commit, buildDate string) string {
	return fmt.Sprintf("pico-editor %s (%s, %s)", buildVersion, commit, buildDate)
}
