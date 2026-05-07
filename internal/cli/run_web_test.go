package cli

import (
	"strings"
	"testing"
)

func TestPrintWebBannerEmitsThreeLineWarning(t *testing.T) {
	var sb strings.Builder
	printWebBanner(&sb)
	got := sb.String()

	want := []string{
		"Thimble web is a SINGLE-OPERATOR LOCAL TOOL.",
		"For shared/production workflows, use the CLI.",
		"Token is a session cookie; press Ctrl+C to stop.",
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
