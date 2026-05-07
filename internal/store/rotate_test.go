package store_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/store"
)

// TestRotateRegeneratesProvisionedKeys covers K-37 acceptance #3
// happy path: a value originally set with origin=provision gets a
// fresh, different value after `recipient remove --rotate`, while a
// value originally set with origin=set is left alone and surfaced
// in the NeedsAttention list.
func TestRotateRegeneratesProvisionedKeys(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		testRecipientAlice, testRecipientBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "SESSION_SECRET",
		"original-provisioned-token", store.OriginProvision,
	); err != nil {
		t.Fatalf("set provision: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "STRIPE_KEY",
		"sk_live_operator_supplied", store.OriginSet,
	); err != nil {
		t.Fatalf("set operator: %v", err)
	}

	outcome, err := st.RemoveRecipientWithRotation(
		"svc", "prod", testRecipientBob,
		store.RotateOptions{Rotate: true},
	)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if got := strings.Join(outcome.Rotated, ","); got != "SESSION_SECRET" {
		t.Fatalf("rotated = %q, want SESSION_SECRET", got)
	}
	if len(outcome.NeedsAttention) != 1 ||
		outcome.NeedsAttention[0].Key != "STRIPE_KEY" {
		t.Fatalf("needs-attention = %+v", outcome.NeedsAttention)
	}

	values, _, err := st.ReadEnv("svc", "prod")
	if err != nil {
		t.Fatalf("read after rotate: %v", err)
	}
	if values["SESSION_SECRET"] == "original-provisioned-token" {
		t.Fatalf("SESSION_SECRET was not regenerated")
	}
	if values["SESSION_SECRET"] == "" {
		t.Fatalf("SESSION_SECRET is empty after rotation")
	}
	if values["STRIPE_KEY"] != "sk_live_operator_supplied" {
		t.Fatalf("STRIPE_KEY changed: %q", values["STRIPE_KEY"])
	}
}

// TestRotateRandomsOnlySuppressesNeedsAttention covers K-37 #4: the
// silent variant rotates provisioned keys and produces no
// NeedsAttention output. The store layer drops the data and the CLI
// also suppresses the per-key print, so the user sees only the
// rotation summary (or nothing if there are no provisioned keys).
func TestRotateRandomsOnlySuppressesNeedsAttention(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		testRecipientAlice, testRecipientBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "TOKEN", "topsecret", store.OriginSet,
	); err != nil {
		t.Fatalf("set: %v", err)
	}
	outcome, err := st.RemoveRecipientWithRotation(
		"svc", "prod", testRecipientBob,
		store.RotateOptions{Rotate: true, RandomsOnly: true},
	)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if len(outcome.Rotated) != 0 {
		t.Fatalf("rotated = %+v; want empty", outcome.Rotated)
	}
	if len(outcome.NeedsAttention) != 0 {
		t.Fatalf(
			"needs-attention should be empty in randoms-only mode, got %+v",
			outcome.NeedsAttention,
		)
	}
}

// TestRotateMidFlowFailureLeavesStateUnchanged covers K-37 #5
// (atomicity): when origins.json fails to write after the bundle
// has been re-encrypted, the rotation rolls back the bundle and
// origins file to their pre-rotation state. The recipient list,
// values, and origins must all match what they were before
// RemoveRecipientWithRotation was called.
func TestRotateMidFlowFailureLeavesStateUnchanged(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		testRecipientAlice, testRecipientBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	const initialValue = "original-provisioned-token"
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "SESSION_SECRET", initialValue, store.OriginProvision,
	); err != nil {
		t.Fatalf("set: %v", err)
	}
	beforeMeta, err := st.Find("svc", "prod")
	if err != nil {
		t.Fatalf("find before: %v", err)
	}
	beforeValues, _, err := st.ReadEnv("svc", "prod")
	if err != nil {
		t.Fatalf("read before: %v", err)
	}
	beforeOriginsBytes := readOrigins(t, st, "svc", "prod")

	st.ArmFailNextOriginsSave()
	_, err = st.RemoveRecipientWithRotation(
		"svc", "prod", testRecipientBob,
		store.RotateOptions{Rotate: true},
	)
	if err == nil {
		t.Fatal("expected fault-injected error, got nil")
	}
	if !store.IsFaultInjectionOriginsSaveError(err) {
		t.Fatalf("error = %v; want fault-injection sentinel", err)
	}

	afterMeta, err := st.Find("svc", "prod")
	if err != nil {
		t.Fatalf("find after: %v", err)
	}
	if strings.Join(afterMeta.Recipients, ",") !=
		strings.Join(beforeMeta.Recipients, ",") {
		t.Fatalf(
			"recipients changed after rollback: before=%v after=%v",
			beforeMeta.Recipients, afterMeta.Recipients,
		)
	}
	if afterMeta.BundleSHA256 != beforeMeta.BundleSHA256 {
		t.Fatalf(
			"bundle SHA changed after rollback: before=%s after=%s",
			beforeMeta.BundleSHA256, afterMeta.BundleSHA256,
		)
	}
	if afterMeta.Version != beforeMeta.Version {
		t.Fatalf(
			"manifest version changed after rollback: before=%d after=%d",
			beforeMeta.Version, afterMeta.Version,
		)
	}

	afterValues, _, err := st.ReadEnv("svc", "prod")
	if err != nil {
		t.Fatalf("read after: %v", err)
	}
	if afterValues["SESSION_SECRET"] != beforeValues["SESSION_SECRET"] {
		t.Fatalf(
			"SESSION_SECRET changed after rollback: before=%q after=%q",
			beforeValues["SESSION_SECRET"],
			afterValues["SESSION_SECRET"],
		)
	}
	afterOriginsBytes := readOrigins(t, st, "svc", "prod")
	if string(afterOriginsBytes) != string(beforeOriginsBytes) {
		t.Fatalf(
			"origins changed after rollback:\nbefore=%s\nafter=%s",
			beforeOriginsBytes, afterOriginsBytes,
		)
	}
}

// TestRotateAuditEvents covers K-37 #6 final clause: a successful
// rotation appends one recipient_remove event plus one update event
// per rotated key, in that order, with the right Subject fields.
func TestRotateAuditEvents(t *testing.T) {
	st := newTestStore(t)
	st.SetAuditLogger(audit.New(st.Root(), nil))
	if err := st.Init("svc", "prod", []string{
		testRecipientAlice, testRecipientBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "ALPHA", "v1", store.OriginProvision,
	); err != nil {
		t.Fatalf("set alpha: %v", err)
	}
	if err := st.SetSecretWithOrigin(
		"svc", "prod", "BETA", "v2", store.OriginProvision,
	); err != nil {
		t.Fatalf("set beta: %v", err)
	}

	if _, err := st.RemoveRecipientWithRotation(
		"svc", "prod", testRecipientBob,
		store.RotateOptions{Rotate: true},
	); err != nil {
		t.Fatalf("rotate: %v", err)
	}

	events, err := audit.Read(st.Root())
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if len(events) < 3 {
		t.Fatalf("got %d audit events; want >=3 (init, two sets, remove, two updates)", len(events))
	}
	last := events[len(events)-3:]
	wantRemoveSubject := audit.Thumbprint(testRecipientBob)
	if last[0].Op != audit.OpRecipientRemove ||
		last[0].Subject != wantRemoveSubject {
		t.Fatalf("event[-3] = %+v; want recipient_remove of bob", last[0])
	}
	for i, ev := range last[1:] {
		if ev.Op != audit.OpUpdate {
			t.Fatalf("event[-%d] op = %q; want update", 2-i, ev.Op)
		}
	}
}

// TestRotateLazyOriginsForLegacyNamespace covers K-37 #7
// (migration): an existing namespace without an origins file behaves
// as if every key has origin=set. `--rotate` finds no provisioned
// keys, so the rotation is a no-op, and the rest of the remove flow
// (drop-recipient + re-encrypt) still succeeds.
func TestRotateLazyOriginsForLegacyNamespace(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{
		testRecipientAlice, testRecipientBob,
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "TOKEN", "v1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	originsPath := filepath.Join(
		st.Root(), "svc", "prod"+store.OriginsFileSuffix,
	)
	if err := removeIfExists(originsPath); err != nil {
		t.Fatalf("remove origins to simulate legacy: %v", err)
	}

	outcome, err := st.RemoveRecipientWithRotation(
		"svc", "prod", testRecipientBob,
		store.RotateOptions{Rotate: true},
	)
	if err != nil {
		t.Fatalf("rotate legacy: %v", err)
	}
	if len(outcome.Rotated) != 0 {
		t.Fatalf("rotated keys on legacy namespace: %+v", outcome.Rotated)
	}
	if len(outcome.NeedsAttention) != 1 {
		t.Fatalf("needs-attention = %+v", outcome.NeedsAttention)
	}
	if outcome.NeedsAttention[0].Origin != store.OriginSet {
		t.Fatalf(
			"needs-attention origin = %q; want set",
			outcome.NeedsAttention[0].Origin,
		)
	}
}
