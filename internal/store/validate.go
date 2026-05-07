package store

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// bech32Recipient matches `age1` + Bech32 charset (RFC 1, no '1' or
// 'b' or 'i' or 'o'). X25519 recipients are 62 chars in practice, but
// we accept a generous 50-90 char range to absorb future variants.
var bech32Recipient = regexp.MustCompile(
	`^age1[023456789acdefghjklmnpqrstuvwxyz]{50,90}$`,
)

// sshBase64 matches the base64 blob in an OpenSSH public key.
var sshBase64 = regexp.MustCompile(`^[A-Za-z0-9+/]+={0,2}$`)

// ValidateName checks that name is a safe filesystem segment. kind is
// used in the error message ("app" or "environment").
func ValidateName(kind, name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf(
			"invalid %s %q; use letters, digits, dot, underscore, or dash",
			kind, name,
		)
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid %s %q", kind, name)
	}
	return nil
}

// ValidateRecipients returns an error if recipients is empty or any
// individual recipient fails ValidateRecipient.
func ValidateRecipients(recipients []string) error {
	_, err := CleanRecipients(recipients)
	return err
}

// CleanRecipients runs CleanRecipient over each entry and returns the
// canonicalized list. The list is rejected if empty.
func CleanRecipients(recipients []string) ([]string, error) {
	if len(recipients) == 0 {
		return nil, errors.New("at least one recipient is required")
	}
	out := make([]string, 0, len(recipients))
	for _, recipient := range recipients {
		cleaned, err := CleanRecipient(recipient)
		if err != nil {
			return nil, err
		}
		out = append(out, cleaned)
	}
	return out, nil
}

// ValidateRecipient accepts exactly two shapes:
//   - `age1` followed by the Bech32 charset (X25519 recipient).
//   - `ssh-ed25519 ` or `ssh-rsa ` followed by a base64 blob and an
//     optional trailing comment.
//
// A single trailing newline is trimmed. Anything else is rejected with
// a precise message that quotes the first 32 characters of the input.
func ValidateRecipient(recipient string) error {
	_, err := CleanRecipient(recipient)
	return err
}

// CleanRecipient returns the canonical, validated form of recipient
// (with a trailing CR/LF removed). It returns an error matching
// ValidateRecipient when the input is not a recognized shape.
func CleanRecipient(recipient string) (string, error) {
	trimmed := strings.TrimRight(recipient, "\r\n")
	if trimmed == "" {
		return "", errors.New("recipient cannot be empty")
	}
	if strings.ContainsAny(trimmed, "\r\n\t") {
		return "", errors.New(
			"recipient cannot contain whitespace other than a single inline space",
		)
	}
	if isAgeRecipient(trimmed) {
		return trimmed, nil
	}
	if isSSHRecipient(trimmed) {
		return trimmed, nil
	}
	return "", fmt.Errorf(
		"expected `age1...` or `ssh-ed25519 ...`/`ssh-rsa ...`; got `%s`",
		truncate(trimmed, 32),
	)
}

func isAgeRecipient(s string) bool {
	if !strings.HasPrefix(s, "age1") {
		return false
	}
	return bech32Recipient.MatchString(s)
}

func isSSHRecipient(s string) bool {
	for _, prefix := range [...]string{"ssh-ed25519 ", "ssh-rsa "} {
		if strings.HasPrefix(s, prefix) {
			return validateSSHPayload(s[len(prefix):])
		}
	}
	return false
}

// validateSSHPayload checks that the post-prefix part is a base64 blob
// optionally followed by whitespace and a comment.
func validateSSHPayload(rest string) bool {
	if rest == "" {
		return false
	}
	parts := strings.SplitN(rest, " ", 2)
	blob := parts[0]
	if blob == "" {
		return false
	}
	return sshBase64.MatchString(blob)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
