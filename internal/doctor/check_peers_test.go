package doctor

import (
	"strings"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/peer"
	"github.com/cartine/thimble/internal/store"
)

// mustStore returns a Store rooted at root that is good enough for
// the peers check (which only reads .peer-state.json and
// thimble.peers.toml; it does not call into age).
func mustStore(t *testing.T, root string) *store.Store {
	t.Helper()
	return store.New(root, "")
}

// TestBuildPeerCheckResultsAllOK reports green when every peer's
// last_seen is recent and last_error is empty.
func TestBuildPeerCheckResultsAllOK(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	peers := []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
		{Name: "bob", Target: "bob@host.local:/srv/abc-secrets"},
	}
	state := peer.State{Version: 1, Peers: map[string]peer.PeerHealth{
		"alice": {LastSeen: now.Add(-5 * time.Minute)},
		"bob":   {LastSeen: now.Add(-30 * time.Minute)},
	}}
	results := buildPeerCheckResults(peers, state, now)
	if len(results) != 3 {
		t.Fatalf("expected 3 results (summary + 2), got %d", len(results))
	}
	if results[0].Name != "peers" || results[0].Status != StatusOK {
		t.Fatalf("summary = %+v", results[0])
	}
	for _, r := range results[1:] {
		if r.Status != StatusOK {
			t.Errorf("expected OK for %q, got %s", r.Name, r.Status)
		}
	}
}

// TestBuildPeerCheckResultsStale reports warn for any peer older
// than 1h.
func TestBuildPeerCheckResultsStale(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	peers := []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
	}
	state := peer.State{Version: 1, Peers: map[string]peer.PeerHealth{
		"alice": {LastSeen: now.Add(-2 * time.Hour)},
	}}
	results := buildPeerCheckResults(peers, state, now)
	if results[0].Status != StatusWarn {
		t.Fatalf("summary status = %s; want warn", results[0].Status)
	}
	if !strings.Contains(results[1].Detail, "stale") {
		t.Fatalf("expected stale detail, got %q", results[1].Detail)
	}
}

// TestBuildPeerCheckResultsNeverContacted reports fail when a peer
// has no last_seen and no last_error.
func TestBuildPeerCheckResultsNeverContacted(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	peers := []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
	}
	state := peer.State{Version: 1, Peers: map[string]peer.PeerHealth{}}
	results := buildPeerCheckResults(peers, state, now)
	if results[0].Status != StatusFail {
		t.Fatalf("summary status = %s; want fail", results[0].Status)
	}
	if !strings.Contains(results[1].Detail, "never contacted") {
		t.Fatalf("expected never-contacted detail, got %q", results[1].Detail)
	}
}

// TestBuildPeerCheckResultsLastError reports fail when last_error
// is non-empty.
func TestBuildPeerCheckResultsLastError(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	peers := []peer.Peer{
		{Name: "alice", Target: "alice@host.local:/srv/abc-secrets"},
	}
	state := peer.State{Version: 1, Peers: map[string]peer.PeerHealth{
		"alice": {
			LastSeen:  now.Add(-2 * time.Hour),
			LastError: "ssh: connection refused",
		},
	}}
	results := buildPeerCheckResults(peers, state, now)
	if results[0].Status != StatusFail {
		t.Fatalf("summary status = %s; want fail", results[0].Status)
	}
	if !strings.Contains(results[1].Detail, "ssh") {
		t.Fatalf("expected ssh-error detail, got %q", results[1].Detail)
	}
}

// TestCheckPeersSkipsWhenNoPeers reports OK with the
// "single-leader mode" detail line.
func TestCheckPeersSkipsWhenNoPeers(t *testing.T) {
	root := t.TempDir()
	st := mustStore(t, root)
	results := checkPeers(st)
	if len(results) != 1 {
		t.Fatalf("expected single result, got %d", len(results))
	}
	if results[0].Status != StatusOK {
		t.Fatalf("status = %s; want ok", results[0].Status)
	}
	if !strings.Contains(results[0].Detail, "single-leader") {
		t.Fatalf("expected single-leader detail, got %q", results[0].Detail)
	}
}

// TestWorsten covers the rollup ordering helper.
func TestWorsten(t *testing.T) {
	cases := []struct {
		a, b, want Status
	}{
		{StatusOK, StatusOK, StatusOK},
		{StatusOK, StatusWarn, StatusWarn},
		{StatusWarn, StatusOK, StatusWarn},
		{StatusOK, StatusFail, StatusFail},
		{StatusWarn, StatusFail, StatusFail},
		{StatusFail, StatusOK, StatusFail},
	}
	for _, c := range cases {
		if got := worsten(c.a, c.b); got != c.want {
			t.Errorf("worsten(%s, %s) = %s; want %s", c.a, c.b, got, c.want)
		}
	}
}
