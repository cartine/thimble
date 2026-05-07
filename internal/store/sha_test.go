package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRoundTripPopulatesBundleSHA covers K-22 #1: every encrypt-and-
// write stores sha256(ciphertext) in the manifest entry.
func TestRoundTripPopulatesBundleSHA(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	meta, err := st.Find("svc", "prod")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if meta.BundleSHA256 == "" {
		t.Fatalf("init did not populate BundleSHA256")
	}
	bundlePath := filepath.Join(
		st.Root(), filepath.FromSlash(meta.File),
	)
	got, err := readSHA(bundlePath)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	if got != meta.BundleSHA256 {
		t.Fatalf(
			"manifest sha=%s, file sha=%s; want match",
			meta.BundleSHA256, got,
		)
	}

	if err := st.SetSecret("svc", "prod", "K", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	meta2, err := st.Find("svc", "prod")
	if err != nil {
		t.Fatalf("find after set: %v", err)
	}
	if meta2.BundleSHA256 == meta.BundleSHA256 {
		t.Fatalf("BundleSHA256 unchanged after set; want bump")
	}
}

// TestTamperedBundleRejected covers K-22 #2: editing the on-disk
// ciphertext after a successful write must cause the next decrypt to
// be rejected before age runs.
func TestTamperedBundleRejected(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "K", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	bundlePath := filepath.Join(st.Root(), "svc", "prod.env.age")
	tamperBundle(t, bundlePath)
	_, err := st.Render("svc", "prod")
	if err == nil {
		t.Fatalf("Render after tamper succeeded; want SHA-256 mismatch error")
	}
	if !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("error %v missing SHA-256 mismatch text", err)
	}
}

// TestEmptyBundleSHAUpgradesOnRewrite covers K-22 #3: a manifest
// missing BundleSHA256 (older format) is upgraded silently on the
// first decrypt-and-rewrite cycle, with a one-time stderr note.
func TestEmptyBundleSHAUpgradesOnRewrite(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := st.SetSecret("svc", "prod", "K", "v"); err != nil {
		t.Fatalf("set: %v", err)
	}
	zeroOutSHA(t, st.Root(), "svc", "prod")

	var notice strings.Builder
	st.SetNoticeWriter(&notice)
	// A read-only render must NOT error on empty SHA (per spec):
	if _, err := st.Render("svc", "prod"); err != nil {
		t.Fatalf("Render on empty-SHA manifest errored: %v", err)
	}
	if notice.Len() != 0 {
		t.Fatalf("Render emitted upgrade note: %q", notice.String())
	}
	// A mutating call upgrades and announces:
	if err := st.SetSecret("svc", "prod", "K2", "v2"); err != nil {
		t.Fatalf("set after empty SHA: %v", err)
	}
	if !strings.Contains(notice.String(), "BundleSHA256 was empty") {
		t.Fatalf("missing upgrade note; got %q", notice.String())
	}
	meta, err := st.Find("svc", "prod")
	if err != nil {
		t.Fatalf("find after upgrade: %v", err)
	}
	if meta.BundleSHA256 == "" {
		t.Fatalf("BundleSHA256 not populated after rewrite")
	}
	// Re-rendering after upgrade is silent:
	notice.Reset()
	if _, err := st.Render("svc", "prod"); err != nil {
		t.Fatalf("Render after upgrade: %v", err)
	}
	if notice.Len() != 0 {
		t.Fatalf("post-upgrade Render re-announced: %q", notice.String())
	}
}

// TestVerifyReportsRecipientPrefixes covers K-22 #4: thimble verify
// returns prefixes alongside the recipient values.
func TestVerifyReportsRecipientPrefixes(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	report, err := st.Verify("svc", "prod")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !report.Match {
		t.Fatalf("fresh init verdict = mismatch; want match")
	}
	if len(report.Recipients) != 1 {
		t.Fatalf("recipients len = %d, want 1", len(report.Recipients))
	}
	if report.Recipients[0].Prefix != "age1" {
		t.Fatalf("prefix = %q, want age1", report.Recipients[0].Prefix)
	}
	if report.BundleMode.Perm() != 0o600 {
		t.Fatalf("bundle mode = %#o, want 0600", report.BundleMode.Perm())
	}
}

// TestVerifyDetectsTamper covers K-22's verify side: a tampered
// bundle is reported with verdict MISMATCH and report.Match == false.
func TestVerifyDetectsTamper(t *testing.T) {
	st := newTestStore(t)
	if err := st.Init("svc", "prod", []string{testRecipientAlice}); err != nil {
		t.Fatalf("init: %v", err)
	}
	bundlePath := filepath.Join(st.Root(), "svc", "prod.env.age")
	tamperBundle(t, bundlePath)
	report, err := st.Verify("svc", "prod")
	if err != nil {
		t.Fatalf("Verify after tamper: %v", err)
	}
	if report.Match {
		t.Fatalf("Match=true after tamper")
	}
	if report.ManifestSHA == report.ActualSHA {
		t.Fatalf("manifest sha == actual sha after tamper")
	}
}

func readSHA(path string) (string, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- test-only path.
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func tamperBundle(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path) // #nosec G304 -- test-only path.
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	b = append(b, 'X')
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatalf("tamper write: %v", err)
	}
}

// zeroOutSHA rewrites the manifest in place with BundleSHA256
// stripped out of (app, env), simulating an older Thimble write.
// It uses a generic JSON walk so the test does not depend on the
// manifest's pretty-printed line layout.
func zeroOutSHA(t *testing.T, root, app, env string) {
	t.Helper()
	mf := filepath.Join(root, "thimble.json")
	b, err := os.ReadFile(mf) // #nosec G304 -- test-only path.
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	apps, _ := raw["apps"].(map[string]interface{})
	a, _ := apps[app].(map[string]interface{})
	envs, _ := a["environments"].(map[string]interface{})
	e, _ := envs[env].(map[string]interface{})
	delete(e, "bundle_sha256")
	out, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(mf, out, 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
