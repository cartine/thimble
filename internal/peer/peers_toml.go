package peer

import (
	"fmt"
	"strings"
)

// parsePeers reads the small TOML subset Thimble accepts for
// peers.toml: a sequence of [[peers]] tables with name and target
// keys. Comments start with '#'. Strings must be double-quoted. No
// multi-line strings, no escapes.  This mirrors the hand-rolled
// parser used by internal/quorum/policy.go so we do not pull in a
// third-party TOML dependency.
func parsePeers(text string) ([]Peer, error) {
	var (
		peers   []Peer
		current *Peer
		section string
		lineNum int
	)
	for _, raw := range strings.Split(text, "\n") {
		lineNum++
		line := stripComment(raw)
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "[[peers]]" {
			peers = append(peers, Peer{})
			current = &peers[len(peers)-1]
			section = "peers"
			continue
		}
		if strings.HasPrefix(line, "[") {
			return nil, fmt.Errorf("line %d: unknown section %q", lineNum, line)
		}
		if err := parseKV(line, lineNum, section, current); err != nil {
			return nil, err
		}
	}
	return finalizePeers(peers)
}

// finalizePeers checks that every parsed peer has both fields and
// that no two peers share a name. The parser tolerates partial
// entries during streaming; the validation runs once at the end so
// the operator gets one error message per file rather than per line.
func finalizePeers(peers []Peer) ([]Peer, error) {
	seen := map[string]bool{}
	for i, p := range peers {
		if p.Name == "" {
			return nil, fmt.Errorf("peer[%d]: name is empty", i)
		}
		if p.Target == "" {
			return nil, fmt.Errorf("peer %q: target is empty", p.Name)
		}
		if seen[p.Name] {
			return nil, fmt.Errorf("duplicate peer name %q", p.Name)
		}
		seen[p.Name] = true
		if err := ValidatePeer(p); err != nil {
			return nil, err
		}
	}
	return peers, nil
}

// stripComment removes the trailing '# ...' comment from a line.
// Comments are only stripped when not inside a double-quoted string;
// the same rule used by internal/quorum/policy.go's parser.
func stripComment(line string) string {
	if idx := strings.IndexByte(line, '#'); idx >= 0 {
		if !strings.ContainsRune(line[:idx], '"') {
			return line[:idx]
		}
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

// parseKV handles a single key = value assignment under the active
// section. A stray assignment outside any section returns an error so
// typos fail loudly instead of silently being ignored.
func parseKV(line string, lineNum int, section string, current *Peer) error {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return fmt.Errorf("line %d: expected key = value", lineNum)
	}
	key := strings.TrimSpace(line[:eq])
	val := strings.TrimSpace(line[eq+1:])
	if section != "peers" || current == nil {
		return fmt.Errorf("line %d: %q outside [[peers]]", lineNum, key)
	}
	unquoted, err := unquote(val)
	if err != nil {
		return fmt.Errorf("line %d: %s", lineNum, err)
	}
	switch key {
	case "name":
		current.Name = unquoted
	case "target":
		current.Target = unquoted
	default:
		return fmt.Errorf("line %d: unknown [[peers]] key %q", lineNum, key)
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

// encodePeers serializes peers into the canonical [[peers]] form. A
// single trailing newline keeps the file POSIX-clean. Empty input
// emits a single header comment so the file is not zero-byte.
func encodePeers(peers []Peer) string {
	var b strings.Builder
	b.WriteString(
		"# Thimble peers (K-55). Each [[peers]] entry is a leader\n" +
			"# this leader replicates to. peer add/remove update this file.\n",
	)
	for _, p := range peers {
		b.WriteString("\n[[peers]]\n")
		fmt.Fprintf(&b, "name = %q\n", p.Name)
		fmt.Fprintf(&b, "target = %q\n", p.Target)
	}
	return b.String()
}
