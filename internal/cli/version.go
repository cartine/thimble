package cli

import (
	"fmt"
	"io"
	"runtime"
	"runtime/debug"
)

// version, commit, and buildDate are set at link time by the release
// workflow via -ldflags. Local builds without ldflags fall back to
// "dev" + the commit recorded in debug.ReadBuildInfo (if any) so
// `thimble --version` is never silent.
//
//nolint:gochecknoglobals // ldflags-injected runtime metadata
var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

// versionString returns the canonical "thimble vX.Y.Z (commit
// abc1234, built 2026-05-07T14:00:00Z, go1.25.0)" line. Missing
// pieces are filled from debug.ReadBuildInfo or render as "unknown".
func versionString() string {
	v, c, d := version, commit, buildDate
	if c == "" || d == "" {
		c, d = fillFromBuildInfo(c, d)
	}
	if c == "" {
		c = "unknown"
	}
	if d == "" {
		d = "unknown"
	}
	return fmt.Sprintf(
		"thimble v%s (commit %s, built %s, %s)",
		v, c, d, runtime.Version(),
	)
}

// fillFromBuildInfo pulls VCS fields out of debug.ReadBuildInfo so
// developer builds carry useful identity even without ldflags.
func fillFromBuildInfo(c, d string) (string, string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return c, d
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if c == "" && s.Value != "" {
				c = shortCommit(s.Value)
			}
		case "vcs.time":
			if d == "" && s.Value != "" {
				d = s.Value
			}
		}
	}
	return c, d
}

func shortCommit(full string) string {
	if len(full) <= 7 {
		return full
	}
	return full[:7]
}

// runVersion writes the version line to stdout. Used for both
// `thimble --version` and `thimble version`.
func runVersion(stdout io.Writer) error {
	_, err := fmt.Fprintln(stdout, versionString())
	return err
}
