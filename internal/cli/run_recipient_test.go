package cli

import (
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/store"
)

const cliQuorumAlice = "age1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const cliQuorumBob = "age1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"

// TestRunRecipientListShowsThumbprints covers K-36 #3 final clause:
// `recipient list` prints prefix labels and thumbprints, never just
// the raw recipient.
func TestRunRecipientListShowsThumbprints(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{cliQuorumAlice, cliQuorumBob}); err != nil {
		t.Fatalf("init: %v", err)
	}
	var stdout, stderr strings.Builder
	if err := runRecipientV2(st, []string{"list", "svc", "prod"}, &stdout, &stderr); err != nil {
		t.Fatalf("list: %v", err)
	}
	out := stdout.String()
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 {
			t.Fatalf("expected 3 fields per line, got %q", line)
		}
		if fields[0] != "age1" {
			t.Fatalf("prefix = %q; want age1", fields[0])
		}
		if len(fields[1]) != 16 {
			t.Fatalf("thumbprint len = %d; want 16 in %q", len(fields[1]), line)
		}
	}
}

// TestRunRecipientAddBootstrapAtSingleRecipient verifies the CLI
// surface accepts --bootstrap and routes it through the gate.
func TestRunRecipientAddBootstrapAtSingleRecipient(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{cliQuorumAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	var stdout, stderr strings.Builder
	args := []string{"add", "--bootstrap", "svc", "prod", cliQuorumBob}
	if err := runRecipientV2(st, args, &stdout, &stderr); err != nil {
		t.Fatalf("add --bootstrap: %v", err)
	}
	if !strings.Contains(stdout.String(), "added recipient") {
		t.Fatalf("stdout = %q; want 'added recipient'", stdout.String())
	}
}

// TestRunRecipientAddRejectsUnknownFlag protects against typos
// silently becoming positional arguments.
func TestRunRecipientAddRejectsUnknownFlag(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{cliQuorumAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	var stdout, stderr strings.Builder
	err := runRecipientV2(
		st, []string{"add", "--unknown", "svc", "prod", cliQuorumBob},
		&stdout, &stderr,
	)
	if err == nil {
		t.Fatal("unknown flag accepted")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %q; want 'unknown flag'", err)
	}
}

// TestRunRecipientUsageWhenInvoked confirms missing subcommand
// yields the usage banner.
func TestRunRecipientUsageWhenInvoked(t *testing.T) {
	var stdout, stderr strings.Builder
	err := runRecipientV2(newTestStore(t), []string{}, &stdout, &stderr)
	if err == nil {
		t.Fatal("missing subcommand accepted")
	}
	if !strings.Contains(err.Error(), "add|remove|list|sign-add") {
		t.Fatalf("error = %q; want subcommand list", err)
	}
}

// TestRunRecipientRemoveStillWorks ensures the subcommand split
// did not regress the legacy remove behavior.
func TestRunRecipientRemoveStillWorks(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{cliQuorumAlice, cliQuorumBob}); err != nil {
		t.Fatalf("init: %v", err)
	}
	var stdout, stderr strings.Builder
	args := []string{"remove", "svc", "prod", cliQuorumBob}
	if err := runRecipientV2(st, args, &stdout, &stderr); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !strings.Contains(stdout.String(), "removed recipient") {
		t.Fatalf("stdout = %q; want 'removed recipient'", stdout.String())
	}
}

// TestPrintAddOutcomeRendersBothStages exercises the dispatcher
// directly so the prepared/added branches produce different output.
func TestPrintAddOutcomeRendersBothStages(t *testing.T) {
	preparedOps := []store.PolicyOperatorView{
		{Name: "alice", Recipient: cliQuorumAlice},
		{Name: "bob", Recipient: cliQuorumBob},
	}
	prepared := store.AddOutcome{
		Stage:           "prepared",
		QuorumM:         2,
		OperatorsCount:  2,
		NewRecipient:    cliQuorumAlice,
		PolicyOperators: preparedOps,
	}
	var stdout, stderr strings.Builder
	if err := printAddOutcome(prepared, "svc", "prod", &stdout, &stderr); err != nil {
		t.Fatalf("prepared: %v", err)
	}
	if !strings.Contains(stdout.String(), "prepared") {
		t.Fatalf("prepared output = %q; want 'prepared'", stdout.String())
	}
	if !strings.Contains(stdout.String(), "sign-add") {
		t.Fatalf("prepared output missing sign-add hint: %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	added := store.AddOutcome{
		Stage:        "added",
		SignerNames:  []string{"alice", "bob"},
		NewRecipient: cliQuorumAlice,
	}
	if err := printAddOutcome(added, "svc", "prod", &stdout, &stderr); err != nil {
		t.Fatalf("added: %v", err)
	}
	if !strings.Contains(stdout.String(), "quorum satisfied") {
		t.Fatalf("added output missing quorum line: %q", stdout.String())
	}
}
