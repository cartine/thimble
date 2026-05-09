package peer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StateFileName is the on-disk filename of the peer health state
// file relative to the store root. K-56 writes last_seen / last_error
// after a push; K-57 writes manifest_versions on a heartbeat ping.
// The file is gitignored — it is local health state, not bundle
// content.
const StateFileName = ".peer-state.json"

// PeerHealth captures the result of the most recent contact with a
// peer.  LastSeen and LastError are mutually informative: a non-zero
// LastSeen with a nil LastError means the most recent contact
// succeeded; a non-empty LastError means the most recent contact
// failed (LastSeen may still hold the timestamp of the prior
// success).
type PeerHealth struct {
	LastSeen         time.Time      `json:"last_seen,omitempty"`
	LastError        string         `json:"last_error,omitempty"`
	ManifestVersions map[string]int `json:"manifest_versions,omitempty"`
}

// State is the parsed in-memory representation of .peer-state.json.
// Version is bumped if the schema ever needs to change.
type State struct {
	Version int                   `json:"version"`
	Peers   map[string]PeerHealth `json:"peers"`
}

// stateMutex serializes read-modify-write cycles on the state file
// within a single process. The file is also written atomically via
// rename, so concurrent processes reach a consistent end state even
// without this mutex; the mutex is just defense in depth.
var stateMutex sync.Mutex

// StatePath returns the absolute path of the peer state file under
// storeRoot.
func StatePath(storeRoot string) string {
	return filepath.Join(storeRoot, StateFileName)
}

// LoadState reads the state file under storeRoot. A missing file is
// returned as an empty State (Version=1, Peers={}) so callers don't
// branch on first-run.
func LoadState(storeRoot string) (State, error) {
	path := StatePath(storeRoot)
	// #nosec G304 -- path is the configured store root joined with
	// StateFileName.
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{Version: 1, Peers: map[string]PeerHealth{}}, nil
	}
	if err != nil {
		return State{}, fmt.Errorf("read %s: %w", path, err)
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.Peers == nil {
		s.Peers = map[string]PeerHealth{}
	}
	if s.Version == 0 {
		s.Version = 1
	}
	return s, nil
}

// SaveState writes the state file under storeRoot atomically. The
// file is created with mode 0o640.
func SaveState(storeRoot string, s State) error {
	if s.Version == 0 {
		s.Version = 1
	}
	if s.Peers == nil {
		s.Peers = map[string]PeerHealth{}
	}
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	path := StatePath(storeRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return atomicWrite(path, body, 0o640)
}

// RecordPushSuccess updates the last_seen for peerName to now and
// clears any previous last_error. The manifest_versions field is
// preserved as-is — K-57 owns it.
func RecordPushSuccess(storeRoot, peerName string, now time.Time) error {
	return mutateState(storeRoot, func(s *State) {
		h := s.Peers[peerName]
		h.LastSeen = now.UTC()
		h.LastError = ""
		s.Peers[peerName] = h
	})
}

// RecordPushFailure updates the last_error for peerName to err. The
// last_seen field is left unchanged so a successful prior contact
// remains visible (the operator can tell "never reached" apart from
// "reached yesterday, failed now").
func RecordPushFailure(storeRoot, peerName string, err error) error {
	return mutateState(storeRoot, func(s *State) {
		h := s.Peers[peerName]
		h.LastError = truncateError(err.Error(), 200)
		s.Peers[peerName] = h
	})
}

// RecordPingResult writes the full PeerHealth atomically. K-57 calls
// this after a heartbeat to update last_seen + manifest_versions in
// one shot.
func RecordPingResult(storeRoot, peerName string, h PeerHealth) error {
	return mutateState(storeRoot, func(s *State) {
		s.Peers[peerName] = h
	})
}

// mutateState reads the state file under stateMutex, applies the
// mutator, and writes it back atomically. This is the single
// chokepoint so K-56 and K-57 never tear each other's writes.
func mutateState(storeRoot string, mutator func(*State)) error {
	stateMutex.Lock()
	defer stateMutex.Unlock()
	s, err := LoadState(storeRoot)
	if err != nil {
		return err
	}
	mutator(&s)
	return SaveState(storeRoot, s)
}

// truncateError caps an error message at maxLen runes so a chatty
// rsync transcript doesn't fill the state file. The trailing "…" is
// only appended on truncation so short errors stay verbatim.
func truncateError(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
