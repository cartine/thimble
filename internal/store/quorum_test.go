package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/quorum"
	"github.com/cartine/thimble/internal/store"
)

const (
	quorumOpAlice = "age1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	quorumOpBob   = "age1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
	quorumOpCarol = "age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq"
	quorumNewRcp  = "age1nnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnn"
)

// TestAddRecipientV2BootstrapPathSucceeds covers the K-36 #4
// chicken-and-egg path: with policy present and the namespace
// holding only its single init recipient, --bootstrap is allowed.
func TestAddRecipientV2BootstrapPathSucceeds(t *testing.T) {
	st := newTestStore(t)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	writePolicyFile(t, st.Root(), 2, []string{quorumOpAlice, quorumOpBob, quorumOpCarol})
	if err := st.Init("svc", "prod", []string{quorumOpAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := st.AddRecipientV2(
		"svc", "prod", quorumOpBob,
		store.AddRecipientOptions{Bootstrap: true},
	)
	if err != nil {
		t.Fatalf("bootstrap add: %v", err)
	}
	if out.Stage != "added" {
		t.Fatalf("stage = %q; want added", out.Stage)
	}
	events := readEvents(t, st.Root())
	addEvent := events[len(events)-1]
	if addEvent.Op != audit.OpRecipientAdd {
		t.Fatalf("last event op = %q; want recipient_add", addEvent.Op)
	}
	if !addEvent.Bootstrap {
		t.Fatal("audit event Bootstrap = false; want true")
	}
}

// TestAddRecipientV2BootstrapRejectedAtTwoOrMore covers K-36 #4:
// once the namespace has 2+ recipients --bootstrap is refused with
// guidance pointing at the quorum flow.
func TestAddRecipientV2BootstrapRejectedAtTwoOrMore(t *testing.T) {
	st := newTestStore(t)
	writePolicyFile(t, st.Root(), 2, []string{quorumOpAlice, quorumOpBob, quorumOpCarol})
	if err := st.Init("svc", "prod", []string{quorumOpAlice, quorumOpBob}); err != nil {
		t.Fatalf("init: %v", err)
	}
	_, err := st.AddRecipientV2(
		"svc", "prod", quorumOpCarol,
		store.AddRecipientOptions{Bootstrap: true},
	)
	if err == nil {
		t.Fatal("bootstrap at 2 recipients succeeded; want rejection")
	}
	if !strings.Contains(err.Error(), "--bootstrap rejected") {
		t.Fatalf("error = %q; want '--bootstrap rejected'", err)
	}
}

// TestAddRecipientV2NoPolicyFileBehavesLegacy: with no policy file
// present the add is unmodified pre-K-36 behavior.
func TestAddRecipientV2NoPolicyFileBehavesLegacy(t *testing.T) {
	st := newTestStore(t)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	if err := st.Init("svc", "prod", []string{quorumOpAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	out, err := st.AddRecipientV2(
		"svc", "prod", quorumOpBob, store.AddRecipientOptions{},
	)
	if err != nil {
		t.Fatalf("legacy add: %v", err)
	}
	if out.Stage != "added" {
		t.Fatalf("stage = %q; want added (legacy)", out.Stage)
	}
}

// TestAddRecipientV2RejectsBootstrapWhenPolicyHasMatchingNew is a
// guardrail: a maintainer can't sneak the new recipient into the
// policy file as one of the operators (which would imply they
// already control its key) and then bootstrap.
func TestAddRecipientV2QuorumPathFlow(t *testing.T) {
	st := newPolicyTestStore(t, quorumOpAlice)
	if err := st.Init("svc", "prod", []string{quorumOpAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "TOKEN", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	writePolicyFile(t, st.Root(), 2, []string{quorumOpAlice, quorumOpBob, quorumOpCarol})

	// Phase 1: prepare. Maintainer is alice (we configured that
	// identity above). Add a third recipient as the new add.
	out, err := st.AddRecipientV2(
		"svc", "prod", quorumNewRcp, store.AddRecipientOptions{},
	)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if out.Stage != "prepared" {
		t.Fatalf("stage = %q; want prepared", out.Stage)
	}

	// Two operators sign-add. Each operator runs with their own
	// identity; we re-use the single fake-age script here by
	// rewriting its identity claim (the fake age decrypts whatever
	// it can read; the test below sets the right env for each pass).
	signAsOperator(t, st.Root(), "svc", "prod", quorumOpAlice)
	signAsOperator(t, st.Root(), "svc", "prod", quorumOpBob)

	// Phase 2: maintainer re-runs add. With M=2 sigs and verifier
	// identity present, this should commit.
	out, err = st.AddRecipientV2(
		"svc", "prod", quorumNewRcp, store.AddRecipientOptions{},
	)
	if err != nil {
		t.Fatalf("verify+commit: %v", err)
	}
	if out.Stage != "added" {
		t.Fatalf("stage = %q; want added", out.Stage)
	}
	if len(out.SignerThumbs) != 2 {
		t.Fatalf("signers = %d; want 2", len(out.SignerThumbs))
	}
	// Pending dir is cleared on success.
	pendingMeta := filepath.Join(
		st.Root(), ".pending-recipient-adds", "meta.json",
	)
	if _, err := os.Stat(pendingMeta); !os.IsNotExist(err) {
		t.Fatalf("pending dir not cleaned: stat = %v", err)
	}
}

// TestAddRecipientV2QuorumShortFails locks in the missing-operator
// error message: 1 of M=2 collected, error must list the remaining.
func TestAddRecipientV2QuorumShortFails(t *testing.T) {
	st := newPolicyTestStore(t, quorumOpAlice)
	if err := st.Init("svc", "prod", []string{quorumOpAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	writePolicyFile(t, st.Root(), 2, []string{quorumOpAlice, quorumOpBob, quorumOpCarol})

	out, err := st.AddRecipientV2(
		"svc", "prod", quorumNewRcp, store.AddRecipientOptions{},
	)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if out.Stage != "prepared" {
		t.Fatalf("stage = %q; want prepared", out.Stage)
	}
	signAsOperator(t, st.Root(), "svc", "prod", quorumOpAlice)
	_, err = st.AddRecipientV2(
		"svc", "prod", quorumNewRcp, store.AddRecipientOptions{},
	)
	if err == nil {
		t.Fatal("verify with 1 sig succeeded; want short error")
	}
	if !strings.Contains(err.Error(), "1/2") {
		t.Fatalf("error = %q; want 1/2", err)
	}
}

// TestQuorumAuditCapturesSigners covers K-36 #5 final clause: the
// audit log entry for the gated recipient_add records the signers'
// thumbprints, never the recipient strings themselves.
func TestQuorumAuditCapturesSigners(t *testing.T) {
	st := newPolicyTestStore(t, quorumOpAlice)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	if err := st.Init("svc", "prod", []string{quorumOpAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	writePolicyFile(t, st.Root(), 2, []string{quorumOpAlice, quorumOpBob, quorumOpCarol})

	addOpts := store.AddRecipientOptions{}
	if _, err := st.AddRecipientV2("svc", "prod", quorumNewRcp, addOpts); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	signAsOperator(t, st.Root(), "svc", "prod", quorumOpAlice)
	signAsOperator(t, st.Root(), "svc", "prod", quorumOpBob)
	if _, err := st.AddRecipientV2("svc", "prod", quorumNewRcp, addOpts); err != nil {
		t.Fatalf("commit: %v", err)
	}
	events := readEvents(t, st.Root())
	last := events[len(events)-1]
	if last.Op != audit.OpRecipientAdd {
		t.Fatalf("last op = %q; want recipient_add", last.Op)
	}
	if len(last.Signers) != 2 {
		t.Fatalf("signers = %v; want 2", last.Signers)
	}
	for _, s := range last.Signers {
		if len(s) != 16 {
			t.Fatalf("signer thumb %q has len %d; want 16", s, len(s))
		}
	}
	logRaw, err := os.ReadFile(filepath.Join(st.Root(), audit.LogFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	logStr := string(logRaw)
	leakedA := strings.Contains(logStr, quorumOpAlice)
	leakedB := strings.Contains(logStr, quorumOpBob)
	if leakedA || leakedB {
		t.Fatalf("log leaked recipient strings: %s", logRaw)
	}
}

// newPolicyTestStore returns a Store backed by the multi-recipient
// fake age script and configured with maintainerRecipient as the
// CLI's identity (so the verifier address resolves and SignAdd's
// identity check passes when run with the same identity).
func newPolicyTestStore(t *testing.T, maintainerRecipient string) *store.Store {
	t.Helper()
	root := t.TempDir()
	fakeAge := writeMultiFakeAge(t, root)
	idPath := writeFakeIdentity(t, root, maintainerRecipient)
	st := store.New(filepath.Join(root, "secrets"), idPath)
	st.SetAge(age.New(fakeAge, idPath))
	return st
}

// writeFakeIdentity writes an identity file that mimics age-keygen
// output: a comment line carrying the public recipient and a stub
// secret-key line. The secret line is unused by the fake age script
// but must exist so PublicRecipientFromIdentityFile finds the
// `# public key:` header.
func writeFakeIdentity(t *testing.T, root, recipient string) string {
	t.Helper()
	path := filepath.Join(root, "identity-"+recipient[:8]+".txt")
	body := "# created: 2026-05-07T00:00:00Z\n" +
		"# public key: " + recipient + "\n" +
		"AGE-SECRET-KEY-FAKE\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write identity: %v", err)
	}
	return path
}

// writeMultiFakeAge writes a tiny shell that mimics age but in a
// multi-recipient way: encrypt prefixes ciphertext with the joined
// recipient list; decrypt extracts the body if any -i identity file
// names a recipient that matches. Sufficient for the quorum tests.
func writeMultiFakeAge(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "age-multi")
	script := `#!/bin/sh
set -eu
mode="$1"; shift
if [ "$mode" = "-d" ]; then
  identity=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -i) identity="$2"; shift 2 ;;
      *) input="$1"; shift ;;
    esac
  done
  pub=$(grep '^# public key:' "$identity" | head -1 | sed 's/^# public key: //')
  header=$(head -1 "$input")
  body=$(sed '1d' "$input")
  case ",$header," in
    *",$pub,"*) printf '%s' "$body" ;;
    *) echo "no matching recipient" >&2; exit 1 ;;
  esac
else
  recipients=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      -a) shift ;;
      -r)
        if [ -z "$recipients" ]; then
          recipients="$2"
        else
          recipients="$recipients,$2"
        fi
        shift 2
        ;;
      *) shift ;;
    esac
  done
  printf '%s\n' "$recipients"
  cat
fi
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write multi-age: %v", err)
	}
	return path
}

// signAsOperator runs Store.SignAddRecipient with the operator's
// identity configured. Uses a fresh store + age tool pointed at
// the same store root so the signature lands in the shared pending
// dir; the fake age script lives in the parent of the store root.
func signAsOperator(t *testing.T, storeRoot, app, env, operatorRecipient string) {
	t.Helper()
	parent := filepath.Dir(storeRoot)
	idPath := writeFakeIdentity(t, parent, operatorRecipient)
	fakeAge := filepath.Join(parent, "age-multi")
	opStore := store.New(storeRoot, idPath)
	opStore.SetAge(age.New(fakeAge, idPath))
	if _, err := opStore.SignAddRecipient(app, env, quorumNewRcp); err != nil {
		t.Fatalf("sign as %s: %v", operatorRecipient[:8], err)
	}
}

// writePolicyFile writes a recipients.signed.toml under root with
// the given M and operator recipient list.
func writePolicyFile(t *testing.T, storeRoot string, m int, recipients []string) {
	t.Helper()
	if err := os.MkdirAll(storeRoot, 0o700); err != nil {
		t.Fatalf("mkdir store root: %v", err)
	}
	var b strings.Builder
	b.WriteString("[policy]\n")
	b.WriteString("quorum_m = ")
	b.WriteString(itoa(m))
	b.WriteString("\n")
	for i, r := range recipients {
		b.WriteString("\n[[operators]]\nname = \"")
		b.WriteString(operatorName(i))
		b.WriteString("\"\nrecipient = \"")
		b.WriteString(r)
		b.WriteString("\"\n")
	}
	path := filepath.Join(storeRoot, quorum.PolicyFileName)
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var out []byte
	negative := i < 0
	if negative {
		i = -i
	}
	for i > 0 {
		out = append([]byte{byte('0' + i%10)}, out...)
		i /= 10
	}
	if negative {
		return "-" + string(out)
	}
	return string(out)
}

func operatorName(i int) string {
	switch i {
	case 0:
		return "alice"
	case 1:
		return "bob"
	case 2:
		return "carol"
	}
	return "op-" + itoa(i)
}

// readEvents reads and returns all events from the audit log under
// root. Fails the test on any read error.
func readEvents(t *testing.T, root string) []audit.Event {
	t.Helper()
	events, err := audit.Read(root)
	if err != nil {
		t.Fatalf("audit read: %v", err)
	}
	return events
}
