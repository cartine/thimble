package quorum_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/quorum"
)

const newAddRecipient = "age1nnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnnn"
const verifierRecipient = "age1vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv"

// fakeAge is an in-memory AgeTool used by quorum tests. It encrypts
// by tagging plaintext with the recipient (so the plaintext is
// recoverable when "decrypted" with the matching identity); the
// decrypt step only succeeds when the configured identity matches
// one of the encrypted-to recipients.
type fakeAge struct {
	identity   string
	identities []string
	failNext   bool
}

func (f *fakeAge) Encrypt(_ context.Context, recipients []string, plain string) ([]byte, error) {
	if f.failNext {
		f.failNext = false
		return nil, errors.New("forced encrypt failure")
	}
	out := strings.Join(recipients, ",") + "::" + plain
	return []byte(out), nil
}

func (f *fakeAge) Decrypt(_ context.Context, path string) (string, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- test path.
	if err != nil {
		return "", err
	}
	parts := strings.SplitN(string(b), "::", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed fake ciphertext")
	}
	recipients := strings.Split(parts[0], ",")
	for _, r := range recipients {
		if r == f.identity {
			return parts[1], nil
		}
		for _, alt := range f.identities {
			if r == alt {
				return parts[1], nil
			}
		}
	}
	return "", errors.New("fake age: no matching identity")
}

// TestPrepareSignVerifyRoundTrip exercises the full M-of-N flow:
// prepare writes challenges, two operators sign, the third does not,
// verify accepts (M=2).
func TestPrepareSignVerifyRoundTrip(t *testing.T) {
	root := t.TempDir()
	policy := quorum.Policy{
		M: 2,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
			{Name: "bob", Recipient: policyBob},
			{Name: "carol", Recipient: policyCarol},
		},
	}
	verifier := &fakeAge{identity: verifierRecipient}
	if err := quorum.PrepareAdd(context.Background(), quorum.PrepareInputs{
		StoreRoot:         root,
		App:               "svc",
		Env:               "prod",
		NewRecipient:      newAddRecipient,
		BundleSHA:         "sha-pre",
		Policy:            policy,
		VerifierRecipient: verifierRecipient,
		Age:               verifier,
	}); err != nil {
		t.Fatalf("PrepareAdd: %v", err)
	}
	signWith(t, root, "svc", "prod", policyAlice)
	signWith(t, root, "svc", "prod", policyBob)
	res, err := quorum.Verify(context.Background(), quorum.VerifyInputs{
		StoreRoot:    root,
		App:          "svc",
		Env:          "prod",
		NewRecipient: newAddRecipient,
		BundleSHA:    "sha-pre",
		Policy:       policy,
		Age:          verifier,
	})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(res.SignerThumbs) != 2 {
		t.Fatalf("signers = %d; want 2", len(res.SignerThumbs))
	}
}

// TestVerifyShortRejectsAndListsMissing covers the M-of-N short
// case: only 1 of the required 2 signs, and the error names the
// remaining operators.
func TestVerifyShortRejectsAndListsMissing(t *testing.T) {
	root := t.TempDir()
	policy := quorum.Policy{
		M: 2,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
			{Name: "bob", Recipient: policyBob},
		},
	}
	verifier := &fakeAge{identity: verifierRecipient}
	mustPrepare(t, root, policy, verifier)
	signWith(t, root, "svc", "prod", policyAlice)
	_, err := quorum.Verify(context.Background(), quorum.VerifyInputs{
		StoreRoot:    root,
		App:          "svc",
		Env:          "prod",
		NewRecipient: newAddRecipient,
		BundleSHA:    "sha-pre",
		Policy:       policy,
		Age:          verifier,
	})
	if err == nil {
		t.Fatal("Verify short = nil; want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "1/2") {
		t.Fatalf("error = %q; want contains 1/2", msg)
	}
	if !strings.Contains(msg, "bob") {
		t.Fatalf("error = %q; want contains 'bob'", msg)
	}
}

// TestVerifyRejectsNonOperatorSignature simulates a non-listed
// operator producing a signature: the .sig file exists, but its
// thumbprint is not in the policy. Verify must ignore it.
func TestVerifyRejectsNonOperatorSignature(t *testing.T) {
	root := t.TempDir()
	policy := quorum.Policy{
		M: 1,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
		},
	}
	verifier := &fakeAge{identity: verifierRecipient}
	mustPrepare(t, root, policy, verifier)
	const malloryRecipient = "age1mmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmmm"
	malloryThumb := quorum.RecipientThumbprint(malloryRecipient)
	canonical := quorum.CanonicalMessage(
		"svc", "prod", newAddRecipient, "sha-pre", currentNonce(t, root),
	)
	sig := []byte(verifierRecipient + "::" + canonical)
	if err := os.WriteFile(
		quorum.SignaturePath(root, malloryThumb), sig, 0o600,
	); err != nil {
		t.Fatalf("write mallory sig: %v", err)
	}
	_, err := quorum.Verify(context.Background(), quorum.VerifyInputs{
		StoreRoot:    root,
		App:          "svc",
		Env:          "prod",
		NewRecipient: newAddRecipient,
		BundleSHA:    "sha-pre",
		Policy:       policy,
		Age:          verifier,
	})
	if err == nil {
		t.Fatal("Verify with mallory sig = nil; want error")
	}
	if !strings.Contains(err.Error(), "0/1") {
		t.Fatalf("error = %q; want contains 0/1", err)
	}
}

// TestVerifyRejectsTamperedRecipient confirms a signature carrying
// the canonical message for one recipient cannot be replayed when
// the maintainer is finalizing a different recipient. This is the
// anti-substitution invariant.
func TestVerifyRejectsTamperedRecipient(t *testing.T) {
	root := t.TempDir()
	policy := quorum.Policy{
		M: 1,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
		},
	}
	verifier := &fakeAge{identity: verifierRecipient}
	mustPrepare(t, root, policy, verifier)
	signWith(t, root, "svc", "prod", policyAlice)
	const otherRecipient = "age1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	_, err := quorum.Verify(context.Background(), quorum.VerifyInputs{
		StoreRoot:    root,
		App:          "svc",
		Env:          "prod",
		NewRecipient: otherRecipient,
		BundleSHA:    "sha-pre",
		Policy:       policy,
		Age:          verifier,
	})
	if err == nil {
		t.Fatal("Verify with substituted recipient = nil; want error")
	}
	if !strings.Contains(err.Error(), "pending add is for recipient") {
		t.Fatalf("error = %q; want substitution error", err)
	}
}

// TestVerifyRejectsStaleBundleSHA confirms a signature collected
// over an old bundle SHA is rejected once the bundle has been
// re-encrypted.
func TestVerifyRejectsStaleBundleSHA(t *testing.T) {
	root := t.TempDir()
	policy := quorum.Policy{
		M: 1,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
		},
	}
	verifier := &fakeAge{identity: verifierRecipient}
	mustPrepare(t, root, policy, verifier)
	signWith(t, root, "svc", "prod", policyAlice)
	_, err := quorum.Verify(context.Background(), quorum.VerifyInputs{
		StoreRoot:    root,
		App:          "svc",
		Env:          "prod",
		NewRecipient: newAddRecipient,
		BundleSHA:    "sha-changed",
		Policy:       policy,
		Age:          verifier,
	})
	if err == nil {
		t.Fatal("Verify with stale SHA = nil; want error")
	}
	if !strings.Contains(err.Error(), "bundle SHA changed") {
		t.Fatalf("error = %q; want stale-SHA text", err)
	}
}

// mustPrepare runs PrepareAdd against a fixed (svc, prod, newAddRecipient,
// "sha-pre") so individual tests need not repeat the boilerplate.
func mustPrepare(t *testing.T, root string, policy quorum.Policy, age quorum.AgeTool) {
	t.Helper()
	if err := quorum.PrepareAdd(context.Background(), quorum.PrepareInputs{
		StoreRoot:         root,
		App:               "svc",
		Env:               "prod",
		NewRecipient:      newAddRecipient,
		BundleSHA:         "sha-pre",
		Policy:            policy,
		VerifierRecipient: verifierRecipient,
		Age:               age,
	}); err != nil {
		t.Fatalf("PrepareAdd: %v", err)
	}
}

// signWith runs SignAdd as the operator whose recipient is opRec.
// The fakeAge handed to SignAdd has identity = opRec so it can
// decrypt only that operator's challenge.
func signWith(t *testing.T, root, app, env, opRec string) {
	t.Helper()
	tool := &fakeAge{identity: opRec}
	_, err := quorum.SignAdd(context.Background(), quorum.SignAddInputs{
		StoreRoot:         root,
		App:               app,
		Env:               env,
		NewRecipient:      newAddRecipient,
		OperatorRecipient: opRec,
		Age:               tool,
	})
	if err != nil {
		t.Fatalf("SignAdd as %s: %v", opRec, err)
	}
}

// currentNonce reads the pending meta.json and returns the nonce.
// Used by TestVerifyRejectsNonOperatorSignature to construct a
// canonically valid (but unauthorized) ciphertext for mallory.
func currentNonce(t *testing.T, root string) string {
	t.Helper()
	meta, ok, err := quorum.LoadMeta(root)
	if err != nil || !ok {
		t.Fatalf("LoadMeta: ok=%v err=%v", ok, err)
	}
	return meta.Nonce
}

// TestSignAddOutsidePolicyRejected confirms that an operator whose
// recipient is not in the policy gets a clear refusal.
func TestSignAddOutsidePolicyRejected(t *testing.T) {
	root := t.TempDir()
	policy := quorum.Policy{
		M: 1,
		Operators: []quorum.Operator{
			{Name: "alice", Recipient: policyAlice},
		},
	}
	mustPrepare(t, root, policy, &fakeAge{identity: verifierRecipient})
	const outsider = "age1ooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooo"
	_, err := quorum.SignAdd(context.Background(), quorum.SignAddInputs{
		StoreRoot:         root,
		App:               "svc",
		Env:               "prod",
		NewRecipient:      newAddRecipient,
		OperatorRecipient: outsider,
		Age:               &fakeAge{identity: outsider},
	})
	if err == nil {
		t.Fatal("SignAdd by outsider succeeded; want refusal")
	}
	if !strings.Contains(err.Error(), "not in the policy") {
		t.Fatalf("error = %q; want 'not in the policy'", err)
	}
}

// TestPaths confirms the on-disk path helpers stay consistent so
// tests that hand-construct file paths match production layout.
func TestPaths(t *testing.T) {
	root := "/store"
	pendingWant := filepath.Join(root, ".pending-recipient-adds")
	if got := quorum.PendingDir(root); got != pendingWant {
		t.Fatalf("PendingDir = %q; want %q", got, pendingWant)
	}
	thumb := quorum.RecipientThumbprint(policyAlice)
	challengeWant := filepath.Join(pendingWant, thumb+".challenge")
	if got := quorum.ChallengePath(root, thumb); got != challengeWant {
		t.Fatalf("ChallengePath = %q; want %q", got, challengeWant)
	}
}
