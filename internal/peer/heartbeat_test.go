package peer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cartine/thimble/internal/peer"
)

// TestPingHappyPath runs Ping against fake ssh and fake rsync that
// both succeed; we expect the returned PingResult to have a
// non-zero LastSeen and the manifest_versions populated from the
// fake manifest.
func TestPingHappyPath(t *testing.T) {
	dir := t.TempDir()
	writeFakeSSH(t, dir, true)
	manifest := `{"apps":{"abc":{"environments":{"production":{"version":42}}}}}`
	writeFakeRsyncWithManifest(t, dir, manifest)
	t.Setenv("THIMBLE_SSH_BINARY", filepath.Join(dir, "ssh"))
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(dir, "rsync"))

	r := peer.Ping(context.Background(), peer.Peer{
		Name:   "alice",
		Target: "alice@host.local:/srv/abc-secrets",
	})
	if r.Err != nil {
		t.Fatalf("Ping err: %v", r.Err)
	}
	if r.Health.LastSeen.IsZero() {
		t.Fatalf("expected LastSeen non-zero")
	}
	if r.Health.ManifestVersions["abc/production"] != 42 {
		t.Fatalf("missing manifest version: %v", r.Health.ManifestVersions)
	}
}

// TestPingSSHFails reports the ssh failure as the error.
func TestPingSSHFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeSSH(t, dir, false)
	writeFakeRsyncWithManifest(t, dir, `{}`)
	t.Setenv("THIMBLE_SSH_BINARY", filepath.Join(dir, "ssh"))
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(dir, "rsync"))

	r := peer.Ping(context.Background(), peer.Peer{
		Name:   "alice",
		Target: "alice@host.local:/srv/abc-secrets",
	})
	if r.Err == nil {
		t.Fatalf("expected ssh failure, got nil")
	}
}

// TestPingRsyncFails reports the rsync failure.
func TestPingRsyncFails(t *testing.T) {
	dir := t.TempDir()
	writeFakeSSH(t, dir, true)
	rsyncFailScript := `#!/bin/sh
echo "fake rsync failure" >&2
exit 23
`
	if err := os.WriteFile(filepath.Join(dir, "rsync"), []byte(rsyncFailScript), 0o700); err != nil {
		t.Fatalf("write rsync: %v", err)
	}
	t.Setenv("THIMBLE_SSH_BINARY", filepath.Join(dir, "ssh"))
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(dir, "rsync"))

	r := peer.Ping(context.Background(), peer.Peer{
		Name:   "alice",
		Target: "alice@host.local:/srv/abc-secrets",
	})
	if r.Err == nil {
		t.Fatalf("expected rsync failure")
	}
}

// TestPingMalformedTarget returns an error before invoking ssh.
func TestPingMalformedTarget(t *testing.T) {
	r := peer.Ping(context.Background(), peer.Peer{Name: "x", Target: "no-colon"})
	if r.Err == nil {
		t.Fatalf("expected malformed-target error")
	}
}

// TestPingAllConcurrent runs PingAll with 6 peers all using the
// success fake; we expect all 6 results to come back without state
// file corruption (the bounded fan-out should serialize state writes
// outside the workers).
func TestPingAllConcurrent(t *testing.T) {
	dir := t.TempDir()
	writeFakeSSH(t, dir, true)
	manifest := `{"apps":{"abc":{"environments":{"production":{"version":7}}}}}`
	writeFakeRsyncWithManifest(t, dir, manifest)
	t.Setenv("THIMBLE_SSH_BINARY", filepath.Join(dir, "ssh"))
	t.Setenv("THIMBLE_RSYNC_BINARY", filepath.Join(dir, "rsync"))

	root := t.TempDir()
	mgr, err := peer.Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for i := 0; i < 6; i++ {
		p := peer.Peer{
			Name:   "peer" + string(rune('0'+i)),
			Target: "user@host.local:/srv/abc-secrets",
		}
		if err := mgr.Add(p); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	results := peer.PingAll(context.Background(), mgr, root)
	if len(results) != 6 {
		t.Fatalf("expected 6 results, got %d", len(results))
	}
	for name, r := range results {
		if r.Err != nil {
			t.Fatalf("peer %s err: %v", name, r.Err)
		}
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(s.Peers) != 6 {
		t.Fatalf("expected 6 entries in state, got %d", len(s.Peers))
	}
}

// TestPingAllNoPeers is the silent zero case.
func TestPingAllNoPeers(t *testing.T) {
	mgr, err := peer.Load(t.TempDir())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	results := peer.PingAll(context.Background(), mgr, t.TempDir())
	if len(results) != 0 {
		t.Fatalf("expected empty, got %v", results)
	}
}

// writeFakeSSH writes a stub ssh that exits 0 if `succeed` is true,
// else exits non-zero with a stderr error.
func writeFakeSSH(t *testing.T, dir string, succeed bool) {
	t.Helper()
	path := filepath.Join(dir, "ssh")
	var script string
	if succeed {
		script = `#!/bin/sh
exit 0
`
	} else {
		script = `#!/bin/sh
echo "fake ssh: connection refused" >&2
exit 255
`
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake ssh: %v", err)
	}
}

// writeFakeRsyncWithManifest writes a stub rsync that, when invoked
// with a destination as the last positional argument, writes the
// given manifest body to that destination. This mimics
// rsync-fetching a remote thimble.json.
func writeFakeRsyncWithManifest(t *testing.T, dir, manifest string) {
	t.Helper()
	path := filepath.Join(dir, "rsync")
	script := `#!/bin/sh
for last do :; done
cat > "$last" <<'JSON'
` + manifest + `
JSON
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake rsync: %v", err)
	}
}
