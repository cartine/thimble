package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
)

// ThumbprintLen is the number of hex characters we keep from
// sha256(recipient) when forming the operator identifier. 16 hex
// chars (64 bits) is plenty to disambiguate a small operator pool
// while keeping the log line short.
const ThumbprintLen = 16

// UnknownOperator is the placeholder used in audit entries when no
// identity is configured (e.g. during init before any decrypt has
// happened).
const UnknownOperator = "unknown"

// Thumbprint returns sha256(recipient)[:ThumbprintLen] as lowercase
// hex. The recipient string is the public key; this function never
// stores it, so callers can safely log only the return value.
func Thumbprint(recipient string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(recipient)))
	return hex.EncodeToString(sum[:])[:ThumbprintLen]
}

// PublicRecipientFromIdentityFile parses an age identity file and
// returns the `# public key:` comment value. Never returns the
// secret key. age-keygen writes the comment by default; if the
// caller's file lacks it (older format or hand-rolled), an error is
// returned and the caller falls back to UnknownOperator.
func PublicRecipientFromIdentityFile(path string) (string, error) {
	if path == "" {
		return "", errors.New("identity path is empty")
	}
	// #nosec G304 -- path is the operator-supplied identity file;
	// reading it is required to derive the thumbprint.
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	const prefix = "# public key:"
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, prefix) {
			rec := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if rec == "" {
				continue
			}
			return rec, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("identity file lacks `# public key:` comment")
}
