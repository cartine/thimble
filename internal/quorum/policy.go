// Package quorum implements the K-36 M-of-N gate over recipient
// additions. The policy is declared in secrets/recipients.signed.toml
// (a small, intentional TOML subset, not the full spec). Adding a
// recipient when the policy is present requires M signatures from the
// listed operators; the signature mechanism is age's own primitives
// driven by a two-phase challenge / response protocol documented in
// docs/recipient-quorum.md.
//
// This file owns parsing and validating the policy file. It uses a
// hand-rolled parser for the small subset we accept so we do not pull
// in a third-party TOML dependency.
package quorum

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PolicyFileName is the filename Thimble looks for inside the store
// root to discover an active quorum policy. Absence of this file
// keeps the legacy single-operator behavior.
const PolicyFileName = "recipients.signed.toml"

// Operator is one entry in the policy's operators table. Recipient
// is the operator's age public key, used both as the address for the
// challenge (during prepare) and as the policy lookup key (during
// verify). Name is the human-friendly handle used in error messages.
type Operator struct {
	Name      string
	Recipient string
}

// Policy is the parsed in-memory representation of
// recipients.signed.toml. M is the required number of operator
// signatures (1 ≤ M ≤ N where N = len(Operators)). Operators is the
// authoritative list of who may sign.
type Policy struct {
	M         int
	Operators []Operator
}

// PolicyPath returns the absolute path Thimble will probe for the
// quorum policy under storeRoot.
func PolicyPath(storeRoot string) string {
	return filepath.Join(storeRoot, PolicyFileName)
}

// LoadPolicy reads the policy file from path. If the file does not
// exist, (Policy{}, false, nil) is returned and callers should fall
// through to the legacy unsigned path. A read or parse error is
// returned as the third value with present=false.
func LoadPolicy(path string) (Policy, bool, error) {
	// #nosec G304 -- path is the configured store root joined with
	// PolicyFileName; not user input at this layer.
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Policy{}, false, nil
	}
	if err != nil {
		return Policy{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	policy, err := parsePolicy(string(raw))
	if err != nil {
		return Policy{}, false, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := validatePolicy(policy); err != nil {
		return Policy{}, false, fmt.Errorf("validate %s: %w", path, err)
	}
	return policy, true, nil
}

// parsePolicy parses the TOML subset we accept: a top-level [policy]
// table containing quorum_m, and one or more [[operators]] tables
// containing name and recipient. Comments start with `#`. Strings
// must be double-quoted. No multi-line strings, no escapes.
func parsePolicy(text string) (Policy, error) {
	var (
		policy   Policy
		section  string
		current  *Operator
		seenM    bool
		lineNum  int
	)
	for _, raw := range strings.Split(text, "\n") {
		lineNum++
		line := stripComment(raw)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "[[operators]]" {
			policy.Operators = append(policy.Operators, Operator{})
			current = &policy.Operators[len(policy.Operators)-1]
			section = "operators"
			continue
		}
		if line == "[policy]" {
			section = "policy"
			current = nil
			continue
		}
		if strings.HasPrefix(line, "[") {
			return Policy{}, fmt.Errorf("line %d: unknown section %q", lineNum, line)
		}
		if err := parseKV(line, lineNum, section, &policy, current, &seenM); err != nil {
			return Policy{}, err
		}
	}
	if !seenM {
		return Policy{}, errors.New("missing [policy] quorum_m")
	}
	return policy, nil
}

// stripComment removes the trailing `# ...` comment from a line. We
// accept `#` outside quoted strings; double-quoted strings may not
// contain `#` per our subset.
func stripComment(line string) string {
	if idx := strings.IndexByte(line, '#'); idx >= 0 {
		// Only strip if no `"` precedes it on the line.
		if !strings.ContainsRune(line[:idx], '"') {
			return line[:idx]
		}
		// Otherwise look for `#` after the closing quote.
		closeIdx := strings.IndexByte(line[strings.IndexByte(line, '"')+1:], '"')
		if closeIdx >= 0 {
			tail := line[strings.IndexByte(line, '"')+1+closeIdx+1:]
			if hi := strings.IndexByte(tail, '#'); hi >= 0 {
				return line[:strings.IndexByte(line, '"')+1+closeIdx+1+hi]
			}
		}
	}
	return line
}

// parseKV handles a single key=value assignment under the active
// section. Section "" is rejected so a stray assignment outside any
// table does not silently become a policy field.
func parseKV(
	line string, lineNum int, section string,
	policy *Policy, current *Operator, seenM *bool,
) error {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return fmt.Errorf("line %d: expected key = value", lineNum)
	}
	key := strings.TrimSpace(line[:eq])
	val := strings.TrimSpace(line[eq+1:])
	switch section {
	case "policy":
		return parsePolicyKV(key, val, lineNum, policy, seenM)
	case "operators":
		if current == nil {
			return fmt.Errorf("line %d: %q outside [[operators]]", lineNum, key)
		}
		return parseOperatorKV(key, val, lineNum, current)
	default:
		return fmt.Errorf("line %d: %q outside any section", lineNum, key)
	}
}

func parsePolicyKV(key, val string, lineNum int, p *Policy, seenM *bool) error {
	if key != "quorum_m" {
		return fmt.Errorf("line %d: unknown [policy] key %q", lineNum, key)
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return fmt.Errorf("line %d: quorum_m must be integer, got %q", lineNum, val)
	}
	p.M = n
	*seenM = true
	return nil
}

func parseOperatorKV(key, val string, lineNum int, op *Operator) error {
	unquoted, err := unquote(val)
	if err != nil {
		return fmt.Errorf("line %d: %s", lineNum, err)
	}
	switch key {
	case "name":
		op.Name = unquoted
	case "recipient":
		op.Recipient = unquoted
	default:
		return fmt.Errorf("line %d: unknown [[operators]] key %q", lineNum, key)
	}
	return nil
}

// unquote strips surrounding double quotes from a value. Empty values
// or unbalanced quoting return an error so a typo fails loudly.
func unquote(v string) (string, error) {
	if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
		return "", fmt.Errorf("expected double-quoted string, got %q", v)
	}
	inner := v[1 : len(v)-1]
	if strings.ContainsAny(inner, "\"\\") {
		return "", fmt.Errorf("escapes and embedded quotes not supported: %q", v)
	}
	return inner, nil
}

// validatePolicy enforces the K-36 invariants on a parsed policy:
// M ≥ 1, N ≥ M, no duplicate names or recipients. The recipient
// strings themselves are NOT format-checked here; the caller passes
// them to internal/store.ValidateRecipient (importing that here
// would create a cycle).
func validatePolicy(p Policy) error {
	if len(p.Operators) == 0 {
		return errors.New("[[operators]] list is empty")
	}
	if p.M < 1 {
		return fmt.Errorf("[policy] quorum_m = %d; must be ≥ 1", p.M)
	}
	if p.M > len(p.Operators) {
		return fmt.Errorf(
			"[policy] quorum_m = %d exceeds operator count %d",
			p.M, len(p.Operators),
		)
	}
	seenName := map[string]bool{}
	seenRec := map[string]bool{}
	for i, op := range p.Operators {
		if op.Name == "" {
			return fmt.Errorf("operator[%d]: name is empty", i)
		}
		if op.Recipient == "" {
			return fmt.Errorf("operator[%d] %q: recipient is empty", i, op.Name)
		}
		if seenName[op.Name] {
			return fmt.Errorf("duplicate operator name %q", op.Name)
		}
		if seenRec[op.Recipient] {
			return fmt.Errorf(
				"duplicate operator recipient (operator %q)", op.Name,
			)
		}
		seenName[op.Name] = true
		seenRec[op.Recipient] = true
	}
	return nil
}

// FindByRecipient returns the Operator entry whose Recipient equals
// recipient, or (Operator{}, false) if none. Whitespace is trimmed
// before comparison.
func (p Policy) FindByRecipient(recipient string) (Operator, bool) {
	r := strings.TrimSpace(recipient)
	for _, op := range p.Operators {
		if op.Recipient == r {
			return op, true
		}
	}
	return Operator{}, false
}

// Names returns the operator handles in policy order.
func (p Policy) Names() []string {
	out := make([]string, len(p.Operators))
	for i, op := range p.Operators {
		out[i] = op.Name
	}
	return out
}
