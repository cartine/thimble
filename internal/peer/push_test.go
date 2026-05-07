package peer_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/peer"
)

// TestPushChangesNoPeers is the silent zero case.
func TestPushChangesNoPeers(t *testing.T) {
	root := t.TempDir()
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var stderr bytes.Buffer
	failures := peer.PushChanges(context.Background(), mgr, root, &stderr)
	if len(failures) != 0 {
		t.Fatalf("expected no failures, got %v", failures)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected silent push, stderr=%q", stderr.String())
	}
}

// TestPushChangesSuccess runs against a fake rsync that always
// returns 0 and confirms state file is updated.
func TestPushChangesSuccess(t *testing.T) {
	root := t.TempDir()
	mgr := loadWithPeers(t, root, []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
	})
	rsyncDir := writeFakeRsyncForPushTest(t, true)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))

	var stderr bytes.Buffer
	failures := peer.PushChanges(context.Background(), mgr, root, &stderr)
	if len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected silent push, stderr=%q", stderr.String())
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Peers["alice"].LastSeen.IsZero() {
		t.Fatalf("expected last_seen to be set: %+v", s.Peers["alice"])
	}
}

// TestPushChangesFailure surfaces per-peer errors and writes them
// to the state file.
func TestPushChangesFailure(t *testing.T) {
	root := t.TempDir()
	mgr := loadWithPeers(t, root, []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
		{Name: "bob", Target: "bob@host.local:/srv/abc-secrets"},
	})
	rsyncDir := writeFakeRsyncForPushTest(t, false)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))

	var stderr bytes.Buffer
	failures := peer.PushChanges(context.Background(), mgr, root, &stderr)
	if len(failures) != 2 {
		t.Fatalf("expected 2 failures, got %d: %v", len(failures), failures)
	}
	out := stderr.String()
	for _, want := range []string{"alice", "bob", "peer push failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stderr missing %q: %q", want, out)
		}
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Peers["alice"].LastError == "" {
		t.Fatalf("expected last_error for alice")
	}
	if s.Peers["bob"].LastError == "" {
		t.Fatalf("expected last_error for bob")
	}
}

// TestPushChangesPartialFailure exercises one peer succeeding and
// another failing — picked by the fake rsync's host-name parse.
func TestPushChangesPartialFailure(t *testing.T) {
	root := t.TempDir()
	mgr := loadWithPeers(t, root, []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
		{Name: "bob", Target: "bob-fail@host.local:/srv/abc-secrets"},
	})
	rsyncDir := writeFakeRsyncFailOnSubstring(t, "bob-fail")
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))

	var stderr bytes.Buffer
	failures := peer.PushChanges(context.Background(), mgr, root, &stderr)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d: %v", len(failures), failures)
	}
	if failures[0].Peer != "bob" {
		t.Fatalf("expected bob to fail, got %v", failures[0])
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Peers["alice"].LastSeen.IsZero() {
		t.Fatalf("alice should have succeeded")
	}
	if s.Peers["alice"].LastError != "" {
		t.Fatalf("alice should have no error: %q", s.Peers["alice"].LastError)
	}
	if s.Peers["bob"].LastError == "" {
		t.Fatalf("bob should have an error")
	}
}

// TestPushChangesNoRsyncBinary returns nil failures with a stderr
// notice when rsync is not on PATH.
func TestPushChangesNoRsyncBinary(t *testing.T) {
	root := t.TempDir()
	mgr := loadWithPeers(t, root, []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
	})
	t.Setenv("THIMBLE_RSYNC_BINARY", "/no/such/rsync-binary-9d5a")
	var stderr bytes.Buffer
	failures := peer.PushChanges(context.Background(), mgr, root, &stderr)
	if len(failures) != 0 {
		t.Fatalf("expected silent skip, got %v", failures)
	}
	if !strings.Contains(stderr.String(), "peer push skipped") {
		t.Fatalf("expected skip notice, got %q", stderr.String())
	}
}

// TestPushChangesTimeout exercises the per-peer context timeout by
// pinning THIMBLE_PEER_PUSH_TIMEOUT to 1s and using a fake rsync
// that sleeps 5s.
func TestPushChangesTimeout(t *testing.T) {
	root := t.TempDir()
	mgr := loadWithPeers(t, root, []peer.Peer{
		{Name: "slow", Target: "slow@host.local:/srv/abc-secrets"},
	})
	rsyncDir := writeFakeRsyncSlow(t)
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(rsyncDir, "rsync"))
	t.Setenv("THIMBLE_PEER_PUSH_TIMEOUT", "1")
	var stderr bytes.Buffer
	failures := peer.PushChanges(context.Background(), mgr, root, &stderr)
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure (timeout), got %d", len(failures))
	}
}

// loadWithPeers writes a peers.toml with the given peers and returns
// a Manager pointing at it.  Used as the test helper across several
// push tests.
func loadWithPeers(t *testing.T, root string, peers []peer.Peer) *peer.Manager {
	t.Helper()
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, p := range peers {
		if err := mgr.Add(p); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return mgr
}

// writeFakeRsyncForPushTest writes a stub rsync that exits 0 if
// `succeed` is true, else exits 23 with an error message.
func writeFakeRsyncForPushTest(t *testing.T, succeed bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rsync")
	var script string
	if succeed {
		script = `#!/bin/sh
exit 0
`
	} else {
		script = `#!/bin/sh
echo "fake rsync failure: connection refused" >&2
exit 23
`
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
	return dir
}

// writeFakeRsyncFailOnSubstring fails only when any argument
// contains the given substring; otherwise succeeds. Used for
// partial-failure tests.
func writeFakeRsyncFailOnSubstring(t *testing.T, sub string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rsync")
	script := `#!/bin/sh
for a do
  case "$a" in
    *` + sub + `*) echo "fail trigger: $a" >&2; exit 23 ;;
  esac
done
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
	return dir
}

// writeFakeRsyncSlow writes a stub rsync that sleeps before exiting;
// used to validate the per-peer timeout. The exec replaces the shell
// with sleep so context cancellation kills the right pid.
func writeFakeRsyncSlow(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "rsync")
	script := `#!/bin/sh
exec sleep 30
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
	return dir
}
