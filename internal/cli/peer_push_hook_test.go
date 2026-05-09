package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/peer"
)

// TestExtractNoPeerPush confirms the flag is stripped from args
// regardless of position.
func TestExtractNoPeerPush(t *testing.T) {
	cases := []struct {
		name     string
		in       []string
		want     bool
		wantArgs []string
	}{
		{"absent", []string{"a", "b"}, false, []string{"a", "b"}},
		{"flag only", []string{"--no-peer-push"}, true, []string{}},
		{"flag first", []string{"--no-peer-push", "a", "b"}, true, []string{"a", "b"}},
		{"flag mid", []string{"a", "--no-peer-push", "b"}, true, []string{"a", "b"}},
		{"flag last", []string{"a", "b", "--no-peer-push"}, true, []string{"a", "b"}},
		{"explicit true", []string{"a", "--no-peer-push=true"}, true, []string{"a"}},
		{"explicit false", []string{"a", "--no-peer-push=false"}, false, []string{"a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, gotArgs := extractNoPeerPush(c.in)
			if got != c.want {
				t.Fatalf("got %v; want %v", got, c.want)
			}
			if !sliceEqual(gotArgs, c.wantArgs) {
				t.Fatalf("args = %v; want %v", gotArgs, c.wantArgs)
			}
		})
	}
}

// TestPeerPushGloballyDisabled covers the env-var parsing.
func TestPeerPushGloballyDisabled(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"on":    false,
		"true":  false,
		"yes":   false,
		"off":   true,
		"OFF":   true,
		"0":     true,
		"false": true,
		"no":    true,
	}
	for v, want := range cases {
		t.Setenv(peerPushDisableEnv, v)
		if got := peerPushGloballyDisabled(); got != want {
			t.Fatalf("v=%q: got %v; want %v", v, got, want)
		}
	}
}

// TestMaybePushPeersNoPeers exits silently with an empty list.
func TestMaybePushPeersNoPeers(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	if err := os.MkdirAll(cfg.storeDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var stderr bytes.Buffer
	maybePushPeers(cfg, false, &stderr)
	if stderr.Len() != 0 {
		t.Fatalf("expected silent, got %q", stderr.String())
	}
}

// TestMaybePushPeersFiresOnSet runs the public Run() entry against a
// configured peer and confirms the fake rsync was called.
func TestMaybePushPeersFiresOnSet(t *testing.T) {
	store, fakeAge := setupStoreForPushTest(t)
	rsyncDir := setupFakeRsyncMarker(t)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	t.Setenv("THIMBLE_AGE_BINARY", fakeAge)

	cfg := cliConfig{storeDir: store}
	mgr, err := peer.Load(store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := peer.Peer{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"}
	if err := mgr.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var stderr bytes.Buffer
	maybePushPeers(cfg, false, &stderr)
	marker := filepath.Join(rsyncDir, "called")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected fake rsync to have been called: %v", err)
	}
	s, err := peer.LoadState(store)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Peers["alice"].LastSeen.IsZero() {
		t.Fatalf("expected last_seen recorded: %+v", s.Peers["alice"])
	}
}

// TestMaybePushPeersSuppressedFlag confirms --no-peer-push (in
// suppress arg form) skips the push.
func TestMaybePushPeersSuppressedFlag(t *testing.T) {
	store, _ := setupStoreForPushTest(t)
	rsyncDir := setupFakeRsyncMarker(t)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	cfg := cliConfig{storeDir: store}
	mgr, err := peer.Load(store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := peer.Peer{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"}
	if err := mgr.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var stderr bytes.Buffer
	maybePushPeers(cfg, true, &stderr)
	marker := filepath.Join(rsyncDir, "called")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected fake rsync NOT to have been called: %v", err)
	}
}

// TestMaybePushPeersGloballyDisabled confirms THIMBLE_PEER_PUSH=off
// prevents the push.
func TestMaybePushPeersGloballyDisabled(t *testing.T) {
	store, _ := setupStoreForPushTest(t)
	rsyncDir := setupFakeRsyncMarker(t)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	t.Setenv(peerPushDisableEnv, "off")
	cfg := cliConfig{storeDir: store}
	mgr, err := peer.Load(store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := peer.Peer{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"}
	if err := mgr.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var stderr bytes.Buffer
	maybePushPeers(cfg, false, &stderr)
	marker := filepath.Join(rsyncDir, "called")
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected fake rsync NOT to have been called: %v", err)
	}
}

// TestIsMutatingCommandTable is a small table-test for the
// classification helper.
func TestIsMutatingCommandTable(t *testing.T) {
	mutating := []string{"init", "create", "update", "set", "delete", "rm", "and-set", "recipient"}
	readonly := []string{
		"list", "ls", "render", "verify", "audit",
		"doctor", "web", "provision", "and-get", "peer",
	}
	for _, c := range mutating {
		if !isMutatingCommand(c) {
			t.Errorf("isMutatingCommand(%q) = false; want true", c)
		}
	}
	for _, c := range readonly {
		if isMutatingCommand(c) {
			t.Errorf("isMutatingCommand(%q) = true; want false", c)
		}
	}
}

// setupStoreForPushTest creates a temp store dir and a fake age
// binary in the same temp dir. Returns (storeDir, fakeAge path).
func setupStoreForPushTest(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	store := filepath.Join(root, "secrets")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return store, writeFakeAge(t, root)
}

// setupFakeRsyncMarker writes a fake rsync that creates a marker
// file next to itself when invoked. Tests assert on the marker's
// presence.
func setupFakeRsyncMarker(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rsync")
	script := `#!/bin/sh
: >"` + filepath.Join(dir, "called") + `"
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
	return dir
}

// sliceEqual is a tiny helper for slice comparison in
// TestExtractNoPeerPush; we don't pull in cmp for one test.
func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestRunEndToEndPushOnSet exercises the full Run() entry point: an
// init followed by a set, with a peer configured, fires the push
// hook automatically. We assert the fake rsync's marker file
// appears.
func TestRunEndToEndPushOnSet(t *testing.T) {
	root := t.TempDir()
	fakeAge := writeFakeAge(t, root)
	store := filepath.Join(root, "secrets")
	if err := os.MkdirAll(store, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	rsyncDir := setupFakeRsyncMarker(t)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	t.Setenv("THIMBLE_AGE_BINARY", fakeAge)
	t.Setenv("PATH", filepath.Dir(fakeAge)+string(os.PathListSeparator)+os.Getenv("PATH"))

	mgr, err := peer.Load(store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := peer.Peer{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"}
	if err := mgr.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	storeFlag := []string{"--store", store, "--age-binary", fakeAge}
	var stdout, stderr strings.Builder
	initArgs := append([]string{}, storeFlag...)
	initArgs = append(initArgs, "init", "abc", "production", "--recipient", testRecipientOperator)
	if err := Run(initArgs, &stdout, &stderr); err != nil {
		t.Fatalf("init: %v stderr=%s", err, stderr.String())
	}
	marker := filepath.Join(rsyncDir, "called")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected push to fire on init: %v", err)
	}
	// Remove and re-confirm: a `set` with --no-peer-push should NOT
	// recreate the marker.
	if err := os.Remove(marker); err != nil {
		t.Fatalf("remove marker: %v", err)
	}
	setArgs := append([]string{}, storeFlag...)
	setArgs = append(setArgs, "set", "--no-peer-push", "abc", "production", "API_KEY")
	withStdin(t, "value\n", func() {
		if err := Run(setArgs, &stdout, &stderr); err != nil {
			t.Fatalf("set: %v stderr=%s", err, stderr.String())
		}
	})
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("expected --no-peer-push to suppress push: %v", err)
	}
}

// TestMaybePushFailureShowsStderr exercises the failure-flow:
// stderr gets a "peer push failed" line and state file records the
// error.
func TestMaybePushFailureShowsStderr(t *testing.T) {
	store, _ := setupStoreForPushTest(t)
	rsyncDir := writeFakeRsyncCLIFail(t)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	cfg := cliConfig{storeDir: store}
	mgr, err := peer.Load(store)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p := peer.Peer{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"}
	if err := mgr.Add(p); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var stderr bytes.Buffer
	maybePushPeers(cfg, false, &stderr)
	if !strings.Contains(stderr.String(), "peer push failed: alice") {
		t.Fatalf("expected stderr line, got %q", stderr.String())
	}
	s, err := peer.LoadState(store)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Peers["alice"].LastError == "" {
		t.Fatalf("expected last_error: %+v", s.Peers["alice"])
	}
}

// writeFakeRsyncCLIFail writes a fake rsync that fails outright.
func writeFakeRsyncCLIFail(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rsync")
	script := `#!/bin/sh
echo "fake failure: connection refused" >&2
exit 23
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
	return dir
}
