package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/peer"
)

const peerAliceTarget = "alice@laptop.local:/srv/abc-secrets"
const peerBobTarget = "bob@bob-laptop.local:/srv/abc-secrets"

// TestRunPeerAddListRemoveHappyPath exercises the three pure-local
// subcommands end-to-end through runPeer.
func TestRunPeerAddListRemoveHappyPath(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	out, errBuf := bytesPair()

	if err := runPeer(cfg, []string{"add", "alice-laptop", peerAliceTarget}, out, errBuf); err != nil {
		t.Fatalf("peer add: %v", err)
	}
	if !strings.Contains(out.String(), "added peer alice-laptop") {
		t.Fatalf("missing add line, got %q", out.String())
	}
	out.Reset()
	if err := runPeer(cfg, []string{"add", "bob-laptop", peerBobTarget}, out, errBuf); err != nil {
		t.Fatalf("peer add bob: %v", err)
	}
	out.Reset()

	if err := runPeer(cfg, []string{"list"}, out, errBuf); err != nil {
		t.Fatalf("peer list: %v", err)
	}
	listed := out.String()
	for _, want := range []string{"alice-laptop", "bob-laptop", peerAliceTarget, peerBobTarget} {
		if !strings.Contains(listed, want) {
			t.Fatalf("list missing %q: %q", want, listed)
		}
	}

	out.Reset()
	if err := runPeer(cfg, []string{"remove", "alice-laptop"}, out, errBuf); err != nil {
		t.Fatalf("peer remove: %v", err)
	}
	if !strings.Contains(out.String(), "removed peer alice-laptop") {
		t.Fatalf("missing remove line, got %q", out.String())
	}

	mgr, err := peer.Load(cfg.storeDir)
	if err != nil {
		t.Fatalf("Load after CLI: %v", err)
	}
	if got := mgr.List(); len(got) != 1 || got[0].Name != "bob-laptop" {
		t.Fatalf("unexpected post-CLI peers: %+v", got)
	}
}

// TestRunPeerAddDuplicate confirms the second add of the same name
// fails and does NOT corrupt the file.
func TestRunPeerAddDuplicate(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	out, errBuf := bytesPair()
	if err := runPeer(cfg, []string{"add", "alice", peerAliceTarget}, out, errBuf); err != nil {
		t.Fatalf("first add: %v", err)
	}
	err := runPeer(cfg, []string{"add", "alice", peerBobTarget}, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

// TestRunPeerAddBadTarget confirms validation rejects shell-meta in
// the target before the file is touched.
func TestRunPeerAddBadTarget(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"add", "alice", "host:/p;rm"}, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden-character error, got %v", err)
	}
	if _, statErr := os.Stat(peer.PeersPath(cfg.storeDir)); !os.IsNotExist(statErr) {
		t.Fatalf("peers.toml must not exist after rejected add: %v", statErr)
	}
}

// TestRunPeerRemoveMissing surfaces a clear error.
func TestRunPeerRemoveMissing(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"remove", "ghost"}, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not-found, got %v", err)
	}
}

// TestRunPeerListEmpty prints a hint when the file is missing or
// empty.
func TestRunPeerListEmpty(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	out, errBuf := bytesPair()
	if err := runPeer(cfg, []string{"list"}, out, errBuf); err != nil {
		t.Fatalf("peer list: %v", err)
	}
	if !strings.Contains(out.String(), "no peers configured") {
		t.Fatalf("expected hint, got %q", out.String())
	}
}

// TestRunPeerJoinHappyPath runs `peer join` against a fake rsync
// binary on PATH that simulates copying secrets/ from a peer.
func TestRunPeerJoinHappyPath(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	rsyncDir := writeFakeRsync(t, true)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"join", peerAliceTarget}, out, errBuf)
	if err != nil {
		t.Fatalf("peer join: %v\nstderr=%s", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "joined as peer of "+peerAliceTarget) {
		t.Fatalf("missing join line, got %q", out.String())
	}
	// The fake rsync writes a marker into the destination so we can
	// confirm the destination directory was reached.
	marker := filepath.Join(cfg.storeDir, "FAKE_RSYNC_MARKER")
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("expected fake rsync marker, got: %v", err)
	}
}

// TestRunPeerJoinRefuseExisting refuses without --replace when the
// store has a populated thimble.json or any *.env.age bundles.
func TestRunPeerJoinRefuseExisting(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	if err := os.MkdirAll(cfg.storeDir, 0o700); err != nil {
		t.Fatalf("mkdir storeDir: %v", err)
	}
	manifest := filepath.Join(cfg.storeDir, "thimble.json")
	if err := os.WriteFile(manifest, []byte(`{"apps":{"x":{}}}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"join", peerAliceTarget}, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "non-empty") {
		t.Fatalf("expected refusal, got %v", err)
	}
}

// TestRunPeerJoinRefuseExistingBundle covers the bundle-glob branch
// of refuseIfStorePopulated.
func TestRunPeerJoinRefuseExistingBundle(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	if err := os.MkdirAll(filepath.Join(cfg.storeDir, "abc"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bundle := filepath.Join(cfg.storeDir, "abc", "production.env.age")
	if err := os.WriteFile(bundle, []byte("ciphertext"), 0o600); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"join", peerAliceTarget}, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "encrypted bundle") {
		t.Fatalf("expected bundle-refusal, got %v", err)
	}
}

// TestRunPeerJoinReplaceOverrides confirms --replace bypasses the
// populated-store guard.
func TestRunPeerJoinReplaceOverrides(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	if err := os.MkdirAll(cfg.storeDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := filepath.Join(cfg.storeDir, "thimble.json")
	if err := os.WriteFile(manifest, []byte(`{"apps":{"x":{}}}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	rsyncDir := writeFakeRsync(t, true)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"join", "--replace", peerAliceTarget}, out, errBuf)
	if err != nil {
		t.Fatalf("peer join --replace: %v\nstderr=%s", err, errBuf.String())
	}
	if !strings.Contains(out.String(), "joined as peer of") {
		t.Fatalf("missing join line, got %q", out.String())
	}
}

// TestRunPeerJoinRsyncFails surfaces the underlying rsync error.
func TestRunPeerJoinRsyncFails(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	rsyncDir := writeFakeRsync(t, false)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	out, errBuf := bytesPair()
	err := runPeer(cfg, []string{"join", peerAliceTarget}, out, errBuf)
	if err == nil {
		t.Fatalf("expected rsync failure, got nil")
	}
}

// TestRunPeerUsage runs the dispatcher with no subcommand.
func TestRunPeerUsage(t *testing.T) {
	cfg, _ := newPeerConfig(t)
	out, errBuf := bytesPair()
	err := runPeer(cfg, nil, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("expected usage error, got %v", err)
	}
	err = runPeer(cfg, []string{"unknown"}, out, errBuf)
	if err == nil || !strings.Contains(err.Error(), "unknown peer subcommand") {
		t.Fatalf("expected unknown error, got %v", err)
	}
}

// newPeerConfig returns a cliConfig pointing at a fresh temp store
// dir.  The store dir does not yet exist on disk; callers that need
// it pre-existing should MkdirAll themselves.
func newPeerConfig(t *testing.T) (cliConfig, string) {
	t.Helper()
	root := t.TempDir()
	storeDir := filepath.Join(root, "secrets")
	return cliConfig{storeDir: storeDir}, storeDir
}

// bytesPair returns a fresh stdout/stderr pair for capturing CLI
// output.
func bytesPair() (*bytes.Buffer, *bytes.Buffer) {
	return &bytes.Buffer{}, &bytes.Buffer{}
}

// writeFakeRsync writes a temporary rsync wrapper script that either
// succeeds (creating a marker file in the destination) or fails
// outright. The returned directory contains exactly one executable
// named "rsync".
func writeFakeRsync(t *testing.T, succeed bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rsync")
	var script string
	if succeed {
		script = `#!/bin/sh
set -eu
# rsync is invoked as: rsync -av --delete src/ dst/
# The destination is the last positional argument.
for last do :; done
mkdir -p "$last"
: >"$last/FAKE_RSYNC_MARKER"
exit 0
`
	} else {
		script = `#!/bin/sh
echo "fake rsync failure" >&2
exit 23
`
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
	return dir
}
