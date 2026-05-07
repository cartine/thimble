//go:build integration

// Integration variant of the K-36 quorum tests: runs the prepare /
// sign / add round-trip against the real `age` and `age-keygen`
// binaries on PATH. Confirms the protocol works with real X25519
// ciphertext, not just our ROT13 fake.
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

// TestRealAgeQuorumRoundTrip generates 3 real age identities, writes
// a quorum policy file, runs the maintainer-prepare → operator-sign
// → maintainer-commit flow, and asserts the bundle now decrypts under
// the new recipient's identity.
func TestRealAgeQuorumRoundTrip(t *testing.T) {
	requireBinaries(t, "age", "age-keygen")

	root := t.TempDir()
	aliceID, aliceRec := generateIdentity(t, filepath.Join(root, "id-a.txt"))
	_, bobRec := generateIdentity(t, filepath.Join(root, "id-b.txt"))
	_, carolRec := generateIdentity(t, filepath.Join(root, "id-c.txt"))
	newID, newRec := generateIdentity(t, filepath.Join(root, "id-new.txt"))

	storeRoot := filepath.Join(root, "secrets")
	if err := os.MkdirAll(storeRoot, 0o700); err != nil {
		t.Fatalf("mkdir store: %v", err)
	}
	writeRealPolicy(t, storeRoot, 2, []struct{ name, rec string }{
		{"alice", aliceRec}, {"bob", bobRec}, {"carol", carolRec},
	})

	st := store.New(storeRoot, aliceID)
	st.SetAge(age.New("age", aliceID))
	st.SetAuditLogger(audit.New(storeRoot, nil))
	if err := st.Init("svc", "prod", []string{aliceRec}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "TOKEN", "topsecret"); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Phase 1: prepare. Maintainer runs add → returns "prepared".
	out, err := st.AddRecipientV2(
		"svc", "prod", newRec, store.AddRecipientOptions{},
	)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if out.Stage != "prepared" {
		t.Fatalf("stage = %q; want prepared", out.Stage)
	}

	// Phase 2: each operator signs from their own store handle.
	signWithIdentity(t, storeRoot, aliceID, "svc", "prod", newRec)
	signWithIdentity(t, storeRoot, filepath.Join(root, "id-b.txt"), "svc", "prod", newRec)

	// Phase 3: maintainer commits. With M=2 and signers Alice+Bob,
	// the gate accepts and the bundle is re-encrypted.
	out, err = st.AddRecipientV2(
		"svc", "prod", newRec, store.AddRecipientOptions{},
	)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if out.Stage != "added" {
		t.Fatalf("stage = %q; want added", out.Stage)
	}
	if len(out.SignerThumbs) != 2 {
		t.Fatalf("signers = %d; want 2", len(out.SignerThumbs))
	}

	// The bundle must now decrypt under the new recipient's identity.
	newSt := store.New(storeRoot, newID)
	newSt.SetAge(age.New("age", newID))
	rendered, err := newSt.Render("svc", "prod")
	if err != nil {
		t.Fatalf("render under new identity: %v", err)
	}
	if !strings.Contains(rendered, "TOKEN=topsecret") {
		t.Fatalf("rendered missing TOKEN: %q", rendered)
	}
}

// signWithIdentity opens a fresh Store handle pointing at the same
// store root with a different identity, runs sign-add, and fails
// the test on error.
func signWithIdentity(t *testing.T, storeRoot, identity, app, env, newRec string) {
	t.Helper()
	st := store.New(storeRoot, identity)
	st.SetAge(age.New("age", identity))
	if _, err := st.SignAddRecipient(app, env, newRec); err != nil {
		t.Fatalf("sign-add with %s: %v", filepath.Base(identity), err)
	}
}

// writeRealPolicy writes a recipients.signed.toml file under
// storeRoot for the integration test. M is the quorum count; ops
// is the policy operators list with hand-supplied names.
func writeRealPolicy(t *testing.T, storeRoot string, m int, ops []struct{ name, rec string }) {
	t.Helper()
	var b strings.Builder
	b.WriteString("[policy]\nquorum_m = ")
	b.WriteString(itoaInt(m))
	b.WriteString("\n")
	for _, op := range ops {
		b.WriteString("\n[[operators]]\nname = \"")
		b.WriteString(op.name)
		b.WriteString("\"\nrecipient = \"")
		b.WriteString(op.rec)
		b.WriteString("\"\n")
	}
	if err := os.WriteFile(
		filepath.Join(storeRoot, quorum.PolicyFileName),
		[]byte(b.String()), 0o600,
	); err != nil {
		t.Fatalf("write policy: %v", err)
	}
}

func itoaInt(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}
