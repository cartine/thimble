package cli

import (
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/store"
)

// TestRunRecipientRemoveRotateRegeneratesProvisionedKey covers the
// CLI surface of K-37: `recipient remove --rotate` re-encrypts and
// regenerates every value with origin=provision. The "rotated"
// summary line names the affected key and the operator-supplied key
// is surfaced via "manual rotate needed".
func TestRunRecipientRemoveRotateRegeneratesProvisionedKey(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		cliQuorumAlice, cliQuorumBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "SESSION_SECRET",
		"original-token", store.OriginProvision,
	); err != nil {
		t.Fatalf("set provisioned: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "STRIPE_KEY",
		"sk_live_op", store.OriginSet,
	); err != nil {
		t.Fatalf("set operator: %v", err)
	}

	var stdout, stderr strings.Builder
	args := []string{"remove", "--rotate", "svc", "prod", cliQuorumBob}
	if err := runRecipientV2(st, args, &stdout, &stderr); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"removed recipient from svc/prod",
		"rotated SESSION_SECRET (provisioned)",
		"manual rotate needed: STRIPE_KEY (origin: set; re-set the value)",
		"1 rotated, 1 needs-attention",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}

// TestRunRecipientRemoveRotateRandomsOnly covers K-37 #4 from the
// CLI surface: --rotate-randoms-only suppresses the manual-rotate
// hints but keeps the rotation summary.
func TestRunRecipientRemoveRotateRandomsOnly(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		cliQuorumAlice, cliQuorumBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "SESSION_SECRET",
		"original", store.OriginProvision,
	); err != nil {
		t.Fatalf("set provisioned: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "STRIPE_KEY", "sk_live", store.OriginSet,
	); err != nil {
		t.Fatalf("set operator: %v", err)
	}

	var stdout, stderr strings.Builder
	args := []string{
		"remove", "--rotate-randoms-only", "svc", "prod", cliQuorumBob,
	}
	if err := runRecipientV2(st, args, &stdout, &stderr); err != nil {
		t.Fatalf("rotate-randoms-only: %v", err)
	}
	out := stdout.String()
	if strings.Contains(out, "manual rotate needed") {
		t.Fatalf("manual-rotate hint leaked in randoms-only mode:\n%s", out)
	}
	if !strings.Contains(out, "rotated SESSION_SECRET (provisioned)") {
		t.Fatalf("rotated line missing:\n%s", out)
	}
}

// TestRunRecipientRemoveWithoutRotateLeavesValuesUntouched verifies
// the legacy behavior is preserved when --rotate is not given. No
// provisioned key gets regenerated and no rotation summary line is
// printed.
func TestRunRecipientRemoveWithoutRotateLeavesValuesUntouched(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		cliQuorumAlice, cliQuorumBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "SESSION_SECRET",
		"original-token", store.OriginProvision,
	); err != nil {
		t.Fatalf("set: %v", err)
	}

	var stdout, stderr strings.Builder
	args := []string{"remove", "svc", "prod", cliQuorumBob}
	if err := runRecipientV2(st, args, &stdout, &stderr); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if strings.Contains(stdout.String(), "rotated") {
		t.Fatalf(
			"unexpected rotation output without --rotate:\n%s",
			stdout.String(),
		)
	}
	values, _, err := st.ReadEnv("svc", "prod")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if values["SESSION_SECRET"] != "original-token" {
		t.Fatalf(
			"value changed without --rotate: %q",
			values["SESSION_SECRET"],
		)
	}
}

// TestRunSetWithOriginFlagStampsOrigin covers the K-37 hidden
// --origin flag on `set`. After running with --origin=provision,
// the namespace's origins file records the key as OriginProvision.
func TestRunSetWithOriginFlagStampsOrigin(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{cliQuorumAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	withStdin(t, "freshly-provisioned-token\n", func() {
		var stdout, stderr strings.Builder
		args := []string{"--origin=provision", "svc", "prod", "TOKEN"}
		if err := runSet(st, args, &stdout, &stderr); err != nil {
			t.Fatalf("set --origin: %v", err)
		}
	})
	// We can't read origins file directly from this package; but a
	// follow-up `recipient remove --rotate` is the canonical way to
	// observe origin metadata. So drive it and confirm the value
	// rotates.
	if err := st.AddRecipient("svc", "prod", cliQuorumBob); err != nil {
		t.Fatalf("add bob: %v", err)
	}
	outcome, err := st.RemoveRecipientWithRotation(
		"svc", "prod", cliQuorumBob,
		store.RotateOptions{Rotate: true},
	)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if len(outcome.Rotated) != 1 || outcome.Rotated[0] != "TOKEN" {
		t.Fatalf("expected TOKEN to be rotated, got %+v", outcome.Rotated)
	}
}

// TestRunSetRejectsUnknownOrigin guards against typos like
// --origin=provisioned (with the trailing -ed) silently being
// accepted.
func TestRunSetRejectsUnknownOrigin(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{cliQuorumAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	withStdin(t, "value\n", func() {
		var stdout, stderr strings.Builder
		args := []string{"--origin=mystery", "svc", "prod", "TOKEN"}
		err := runSet(st, args, &stdout, &stderr)
		if err == nil {
			t.Fatal("unknown origin accepted")
		}
		if !strings.Contains(err.Error(), "unknown origin") {
			t.Fatalf("error = %v; want unknown origin", err)
		}
	})
}

// TestRunRecipientRemoveRejectsUnknownFlag guards against typos.
func TestRunRecipientRemoveRejectsUnknownFlag(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		cliQuorumAlice, cliQuorumBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	var stdout, stderr strings.Builder
	args := []string{
		"remove", "--rotate-everything", "svc", "prod", cliQuorumBob,
	}
	err := runRecipientV2(st, args, &stdout, &stderr)
	if err == nil {
		t.Fatal("unknown flag accepted")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("error = %v; want unknown flag", err)
	}
}
