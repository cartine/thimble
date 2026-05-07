package cli

import (
	"strings"
	"testing"
	"time"
)

func TestPrintWebBannerEmitsThreeLineWarning(t *testing.T) {
	var sb strings.Builder
	printWebBanner(&sb, 15*time.Minute)
	got := sb.String()

	want := []string{
		"Thimble web is a SINGLE-OPERATOR LOCAL TOOL.",
		"For shared/production workflows, use the CLI.",
		"Token rotates after 15m0s; SIGUSR1 to rotate sooner; Ctrl+C to stop.",
	}
	for _, line := range want {
		if !strings.Contains(got, line) {
			t.Fatalf("banner missing line %q\nfull output:\n%s", line, got)
		}
	}
	if got != strings.Join(want, "\n")+"\n" {
		t.Fatalf("banner format wrong:\n%q\nwant the three lines, each newline-terminated",
			got)
	}
}

func TestPrintWebBannerInterpolatesIdleRotate(t *testing.T) {
	var sb strings.Builder
	printWebBanner(&sb, 30*time.Second)
	got := sb.String()
	if !strings.Contains(got, "Token rotates after 30s;") {
		t.Fatalf("banner did not interpolate --idle-rotate value: %q", got)
	}
}
