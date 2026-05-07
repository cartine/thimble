package store

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

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
	if len(recipients) == 0 {
		return errors.New("at least one recipient is required")
	}
	for _, recipient := range recipients {
		if err := ValidateRecipient(recipient); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRecipient rejects empty, padded, or whitespace-containing
// recipient strings before they reach the age binary.
func ValidateRecipient(recipient string) error {
	if strings.TrimSpace(recipient) != recipient || recipient == "" {
		return errors.New("recipient cannot be empty or padded")
	}
	if strings.ContainsAny(recipient, "\r\n\t ") {
		return errors.New("recipient cannot contain whitespace")
	}
	return nil
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
