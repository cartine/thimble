package quorum_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/quorum"
)

const policyAlice = "age1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const policyBob = "age1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
const policyCarol = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"

// TestLoadPolicyHappyPath confirms a well-formed policy file parses
// into the expected Policy value.
func TestLoadPolicyHappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recipients.signed.toml")
	body := `# K-36 quorum policy
[policy]
quorum_m = 2

[[operators]]
name = "alice"
recipient = "` + policyAlice + `"

[[operators]]
name = "bob"
recipient = "` + policyBob + `"

[[operators]]
name = "carol"
recipient = "` + policyCarol + `"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	policy, present, err := quorum.LoadPolicy(path)
	if err != nil {
		t.Fatalf("LoadPolicy: %v", err)
	}
	if !present {
		t.Fatal("present = false; want true")
	}
	if policy.M != 2 {
		t.Fatalf("M = %d; want 2", policy.M)
	}
	if len(policy.Operators) != 3 {
		t.Fatalf("operators = %d; want 3", len(policy.Operators))
	}
	if policy.Operators[0].Name != "alice" {
		t.Fatalf("operator[0] = %q; want alice", policy.Operators[0].Name)
	}
}

// TestLoadPolicyAbsent confirms a missing file returns (zero, false,
// nil) so callers can fall through to the legacy unsigned path.
func TestLoadPolicyAbsent(t *testing.T) {
	policy, present, err := quorum.LoadPolicy(
		filepath.Join(t.TempDir(), "no-such-file.toml"),
	)
	if err != nil {
		t.Fatalf("LoadPolicy returned error for missing file: %v", err)
	}
	if present {
		t.Fatal("present = true for missing file")
	}
	_ = policy
}

// TestLoadPolicyValidation runs a table of malformed inputs and
// asserts each returns a meaningful error.
func TestLoadPolicyValidation(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name: "missing quorum_m",
			body: `[[operators]]
name = "alice"
recipient = "` + policyAlice + `"`,
			wantErr: "quorum_m",
		},
		{
			name: "M exceeds N",
			body: `[policy]
quorum_m = 3
[[operators]]
name = "alice"
recipient = "` + policyAlice + `"`,
			wantErr: "exceeds operator count",
		},
		{
			name: "M = 0",
			body: `[policy]
quorum_m = 0
[[operators]]
name = "alice"
recipient = "` + policyAlice + `"`,
			wantErr: "≥ 1",
		},
		{
			name: "duplicate name",
			body: `[policy]
quorum_m = 1
[[operators]]
name = "alice"
recipient = "` + policyAlice + `"
[[operators]]
name = "alice"
recipient = "` + policyBob + `"`,
			wantErr: "duplicate operator name",
		},
		{
			name: "duplicate recipient",
			body: `[policy]
quorum_m = 1
[[operators]]
name = "alice"
recipient = "` + policyAlice + `"
[[operators]]
name = "alice2"
recipient = "` + policyAlice + `"`,
			wantErr: "duplicate operator recipient",
		},
		{
			name: "empty operators list",
			body: `[policy]
quorum_m = 1`,
			wantErr: "empty",
		},
		{
			name:    "unknown section",
			body:    `[bogus]`,
			wantErr: "unknown section",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "policy.toml")
			if err := os.WriteFile(path, []byte(tt.body), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			_, _, err := quorum.LoadPolicy(path)
			if err == nil {
				t.Fatalf("LoadPolicy succeeded; want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q; want contains %q", err, tt.wantErr)
			}
		})
	}
}

// TestPolicyFindByRecipient is the policy lookup helper used by
// SignAdd to map an operator's parsed identity public key to a
// policy entry.
func TestPolicyFindByRecipient(t *testing.T) {
	p := quorum.Policy{
		M: 2,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
			{Name: "bob", Recipient: policyBob},
		},
	}
	got, ok := p.FindByRecipient(policyAlice)
	if !ok {
		t.Fatal("FindByRecipient(alice) = !ok; want ok")
	}
	if got.Name != "alice" {
		t.Fatalf("got.Name = %q; want alice", got.Name)
	}
	if _, ok := p.FindByRecipient("age1nope"); ok {
		t.Fatal("FindByRecipient on missing recipient returned ok")
	}
}
