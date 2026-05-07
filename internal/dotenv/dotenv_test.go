package dotenv_test

import (
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/dotenv"
)

// TestParseLargeValueRoundTrips exercises the K-25 expansion: a
// 64 KiB+1 value (the old default scanner limit) parses cleanly with
// the new buffer ceiling.
func TestParseLargeValueRoundTrips(t *testing.T) {
	const size = 64*1024 + 1
	value := strings.Repeat("x", size)
	input := "FOO=" + value + "\n"
	values, err := dotenv.Parse(input)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := values["FOO"]; got != value {
		t.Fatalf("len(value) = %d, want %d", len(got), size)
	}
}

// TestParseValueAtLimitFailsWithClearMessage confirms the K-25
// ceiling is enforced and the error names the line and the limit.
func TestParseValueAtLimitFailsWithClearMessage(t *testing.T) {
	value := strings.Repeat("x", dotenv.MaxValueBytes+1)
	input := "FOO=" + value + "\n"
	_, err := dotenv.Parse(input)
	if err == nil {
		t.Fatalf("Parse accepted oversized value")
	}
	if !strings.Contains(err.Error(), "1 MiB") {
		t.Fatalf("error = %v, want '1 MiB' message", err)
	}
	if !strings.Contains(err.Error(), "store it as a file") {
		t.Fatalf("error missing remediation hint: %v", err)
	}
}
