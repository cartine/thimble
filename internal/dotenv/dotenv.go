// Package dotenv parses and encodes the dotenv key-value format Thimble
// uses inside encrypted bundles. Trust boundary: this package handles
// plaintext key-value pairs in memory only — it never touches the
// filesystem, never shells out, and never logs values.
package dotenv

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// KeyPattern is the canonical dotenv key syntax: uppercase letters,
// digits, and underscore, not starting with a digit.
var KeyPattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// MaxValueBytes caps the size of a single dotenv line so a runaway
// value (e.g. a piped binary blob) does not peg memory. 1 MiB is the
// K-25 ceiling: enough for huge JWTs, certs, and PEM-encoded keys
// while still bounding worst-case allocation.
const MaxValueBytes = 1 << 20

// ValidateKey returns nil if key matches the dotenv-style uppercase
// naming convention, or an error otherwise.
func ValidateKey(key string) error {
	if !KeyPattern.MatchString(key) {
		return fmt.Errorf("invalid key %q; use dotenv-style uppercase names", key)
	}
	return nil
}

// Parse reads a dotenv-formatted string and returns the key/value map.
// Comments (lines starting with #) and blank lines are ignored.
// Quoted values support \n, \r, \t, \\, and \" escapes. Lines longer
// than MaxValueBytes return a precise "exceeds 1 MiB" error rather
// than the default bufio.ErrTooLong.
func Parse(input string) (map[string]string, error) {
	values := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Buffer(make([]byte, 0, 64*1024), MaxValueBytes)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid dotenv line %d", lineNo)
		}
		key := strings.TrimSpace(parts[0])
		if err := ValidateKey(key); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		value, err := parseValue(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf(
				"value on line %d exceeds 1 MiB; store it as a file or split it",
				lineNo+1,
			)
		}
		return nil, err
	}
	return values, nil
}

func parseValue(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value[0] != '"' {
		if strings.ContainsAny(value, "\r\n") {
			return "", errors.New("unquoted multiline values are not supported")
		}
		return value, nil
	}
	return parseQuotedValue(value)
}

func parseQuotedValue(value string) (string, error) {
	var out strings.Builder
	escaped := false
	for i := 1; i < len(value); i++ {
		ch := value[i]
		if escaped {
			out.WriteByte(unescape(ch))
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			if strings.TrimSpace(value[i+1:]) != "" {
				return "", errors.New("trailing content after quoted value")
			}
			return out.String(), nil
		}
		out.WriteByte(ch)
	}
	return "", errors.New("unterminated quoted value")
}

func unescape(ch byte) byte {
	switch ch {
	case 'n':
		return '\n'
	case 'r':
		return '\r'
	case 't':
		return '\t'
	default:
		return ch
	}
}

// Encode serializes a key/value map as dotenv text with sorted keys and
// values quoted only when they need it.
func Encode(values map[string]string) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	for _, key := range keys {
		out.WriteString(key)
		out.WriteByte('=')
		out.WriteString(QuoteValue(values[key]))
		out.WriteByte('\n')
	}
	return out.String()
}

// QuoteValue returns value as-is when it consists only of "safe"
// characters; otherwise it wraps the value in double quotes and escapes
// the standard control characters.
func QuoteValue(value string) string {
	if value == "" {
		return `""`
	}
	if isUnquotedSafe(value) {
		return value
	}
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	)
	return `"` + replacer.Replace(value) + `"`
}

func isUnquotedSafe(value string) bool {
	for _, r := range value {
		if !isSafeRune(r) {
			return false
		}
	}
	return true
}

func isSafeRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	}
	return strings.ContainsRune("_-./:@%+,=~", r)
}
