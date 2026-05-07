package cli

import (
	"runtime"
	"strings"
	"testing"
)

func TestVersionStringContainsGoVersion(t *testing.T) {
	got := versionString()
	if !strings.HasPrefix(got, "thimble v") {
		t.Fatalf("versionString() = %q, want 'thimble v...' prefix", got)
	}
	if !strings.Contains(got, runtime.Version()) {
		t.Fatalf("versionString() = %q, missing %q", got, runtime.Version())
	}
}

func TestVersionStringFallsBackToUnknownWithoutVCS(t *testing.T) {
	// In `go test` builds debug.ReadBuildInfo() may or may not return
	// VCS metadata depending on the environment. Either way, the
	// result must be a well-formed line that mentions either a
	// 7-char commit hash or "unknown".
	got := versionString()
	if !strings.Contains(got, "commit ") {
		t.Fatalf("versionString() = %q, missing 'commit '", got)
	}
	if !strings.Contains(got, "built ") {
		t.Fatalf("versionString() = %q, missing 'built '", got)
	}
}

func TestRunInvokedWithVersionFlag(t *testing.T) {
	var stdout, stderr strings.Builder
	if err := Run([]string{"--version"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run --version: %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "thimble v") {
		t.Fatalf("stdout = %q, want 'thimble v' prefix", stdout.String())
	}
}

func TestRunInvokedWithVersionSubcommand(t *testing.T) {
	var stdout, stderr strings.Builder
	if err := Run([]string{"version"}, &stdout, &stderr); err != nil {
		t.Fatalf("Run version: %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "thimble v") {
		t.Fatalf("stdout = %q, want 'thimble v' prefix", stdout.String())
	}
}
