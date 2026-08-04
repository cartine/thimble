package passphrase

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestGenerateShapeAndWordSpace(t *testing.T) {
	if len(wordStarts) != 128 || len(wordEnds) != 128 {
		t.Fatalf("word list sizes = %d/%d, want 128/128", len(wordStarts), len(wordEnds))
	}
	for _, entries := range [][]string{wordStarts, wordEnds} {
		seen := make(map[string]bool, len(entries))
		for _, entry := range entries {
			if len(entry) < 4 || seen[entry] {
				t.Fatalf("invalid or duplicate word component %q", entry)
			}
			seen[entry] = true
		}
	}

	phrase, err := generateFrom(bytes.NewReader(make([]byte, 64)))
	if err != nil {
		t.Fatal(err)
	}
	if len(phrase) < MinimumLength {
		t.Fatalf("passphrase length = %d, want >= %d", len(phrase), MinimumLength)
	}
	if parts := strings.Split(phrase, "-"); len(parts) != WordCount {
		t.Fatalf("passphrase = %q, want %d words", phrase, WordCount)
	}
	for _, char := range phrase {
		if (char < 'a' || char > 'z') && char != '-' {
			t.Fatalf("passphrase contains unexpected character %q", char)
		}
	}
}

func TestGenerateReportsRandomnessFailure(t *testing.T) {
	_, err := generateFrom(errorReader{})
	if err == nil || !strings.Contains(err.Error(), "generate passphrase") {
		t.Fatalf("error = %v", err)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("random source unavailable")
}
