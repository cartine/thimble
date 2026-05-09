package peer_test

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cartine/thimble/internal/peer"
)

// TestStateMissingFile returns the empty state.
func TestStateMissingFile(t *testing.T) {
	s, err := peer.LoadState(t.TempDir())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Version != 1 {
		t.Fatalf("Version = %d; want 1", s.Version)
	}
	if len(s.Peers) != 0 {
		t.Fatalf("expected empty peers, got %v", s.Peers)
	}
}

// TestStateRoundTrip exercises Save → Load and confirms timestamps
// survive the JSON encode.
func TestStateRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 5, 7, 10, 23, 45, 0, time.UTC)
	want := peer.State{
		Version: 1,
		Peers: map[string]peer.PeerHealth{
			"alice": {
				LastSeen:         now,
				LastError:        "",
				ManifestVersions: map[string]int{"abc/production": 43},
			},
		},
	}
	if err := peer.SaveState(root, want); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.Version != 1 || len(got.Peers) != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
	h := got.Peers["alice"]
	if !h.LastSeen.Equal(now) {
		t.Fatalf("LastSeen = %v; want %v", h.LastSeen, now)
	}
	if h.ManifestVersions["abc/production"] != 43 {
		t.Fatalf("manifest_versions wrong: %v", h.ManifestVersions)
	}
}

// TestRecordPushSuccess clears any prior error and bumps last_seen.
func TestRecordPushSuccess(t *testing.T) {
	root := t.TempDir()
	failErr := errors.New("ssh: connection refused")
	if err := peer.RecordPushFailure(root, "alice", failErr); err != nil {
		t.Fatalf("RecordPushFailure: %v", err)
	}
	now := time.Date(2026, 5, 7, 11, 0, 0, 0, time.UTC)
	if err := peer.RecordPushSuccess(root, "alice", now); err != nil {
		t.Fatalf("RecordPushSuccess: %v", err)
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	h := s.Peers["alice"]
	if h.LastError != "" {
		t.Fatalf("LastError = %q; expected cleared", h.LastError)
	}
	if !h.LastSeen.Equal(now) {
		t.Fatalf("LastSeen = %v; want %v", h.LastSeen, now)
	}
}

// TestRecordPushFailure preserves a prior LastSeen so the operator
// can tell "never reached" apart from "was up, now down".
func TestRecordPushFailure(t *testing.T) {
	root := t.TempDir()
	prior := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	if err := peer.RecordPushSuccess(root, "alice", prior); err != nil {
		t.Fatalf("RecordPushSuccess: %v", err)
	}
	if err := peer.RecordPushFailure(root, "alice", errors.New("rsync: exit 23")); err != nil {
		t.Fatalf("RecordPushFailure: %v", err)
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	h := s.Peers["alice"]
	if !h.LastSeen.Equal(prior) {
		t.Fatalf("LastSeen overwritten on failure: got %v, want %v", h.LastSeen, prior)
	}
	if !strings.Contains(h.LastError, "rsync") {
		t.Fatalf("LastError missing detail: %q", h.LastError)
	}
}

// TestStateFileMode confirms the on-disk file mode is 0o640.
func TestStateFileMode(t *testing.T) {
	root := t.TempDir()
	if err := peer.RecordPushSuccess(root, "alice", time.Now()); err != nil {
		t.Fatalf("RecordPushSuccess: %v", err)
	}
	info, err := os.Stat(peer.StatePath(root))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("file mode = %o; want 0640", got)
	}
}

// TestRecordPingResultPreservesOthers ensures K-57's ping update for
// peer A leaves peer B's data alone.
func TestRecordPingResultPreservesOthers(t *testing.T) {
	root := t.TempDir()
	bobNow := time.Date(2026, 5, 6, 9, 0, 0, 0, time.UTC)
	if err := peer.RecordPushSuccess(root, "bob", bobNow); err != nil {
		t.Fatalf("RecordPushSuccess bob: %v", err)
	}
	aliceNow := time.Date(2026, 5, 7, 9, 0, 0, 0, time.UTC)
	err := peer.RecordPingResult(root, "alice", peer.PeerHealth{
		LastSeen:         aliceNow,
		ManifestVersions: map[string]int{"abc/production": 12},
	})
	if err != nil {
		t.Fatalf("RecordPingResult: %v", err)
	}
	s, err := peer.LoadState(root)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !s.Peers["bob"].LastSeen.Equal(bobNow) {
		t.Fatalf("bob's LastSeen overwritten: %v", s.Peers["bob"].LastSeen)
	}
	if !s.Peers["alice"].LastSeen.Equal(aliceNow) {
		t.Fatalf("alice's LastSeen wrong: %v", s.Peers["alice"].LastSeen)
	}
}
