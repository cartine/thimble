// Package peer owns the membership layer for Thimble's multi-leader
// replication (K-55). Each leader keeps a local file
// <store>/thimble.peers.toml listing the peers it knows about; on
// mutation Thimble pushes to those peers (K-56) and on a periodic ping
// Thimble records peer health (K-57). The membership file is local —
// it is NOT distributed via the bundle, since each operator may run a
// different network topology.
//
// Peer membership grants RSYNC rights, not DECRYPT rights. Granting a
// peer the ability to decrypt is still a recipient-add operation
// gated by the K-36 quorum policy. Adding a peer is a low-stakes
// operation: a peer that isn't a recipient can pull ciphertext but
// cannot read it.
package peer
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PeersFileName is the on-disk filename of the peers list relative to
// the store root. Public so callers (CLI subcommands, the doctor
// check, the on-mutate hook) resolve the same path.
const PeersFileName = "thimble.peers.toml"

// Peer is one entry in the peers list. Name is a short handle the
// operator chose for the leader; Target is an rsync-compatible
// destination of the form `[user@]host:path` pointing at that peer's
// secrets/ directory.
type Peer struct {
	Name   string
	Target string
}

// Manager wraps the on-disk peers file. Construct with Load; mutate
// via Add/Remove; persist with Save.
type Manager struct {
	path  string
	peers []Peer
}

// PeersPath returns the absolute path Thimble probes for the peers
// list under storeRoot.
func PeersPath(storeRoot string) string {
	return filepath.Join(storeRoot, PeersFileName)
}

// Load reads the peers file from storeRoot. A missing file is treated
// as an empty list (the legacy single-leader configuration).
func Load(storeRoot string) (*Manager, error) {
	path := PeersPath(storeRoot)
	// #nosec G304 -- path is the configured store root joined with
	// PeersFileName; not user input at this layer.
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Manager{path: path}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	peers, err := parsePeers(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &Manager{path: path, peers: peers}, nil
}

// Path returns the absolute file path the Manager persists to.
func (m *Manager) Path() string { return m.path }

// List returns a defensive copy of the peers in declared order.
func (m *Manager) List() []Peer {
	out := make([]Peer, len(m.peers))
	copy(out, m.peers)
	return out
}

// Find returns the peer with the given name and whether it was
// present.
func (m *Manager) Find(name string) (Peer, bool) {
	for _, p := range m.peers {
		if p.Name == name {
			return p, true
		}
	}
	return Peer{}, false
}

// Add appends a peer to the list. The peer's Name and Target are
// validated and a duplicate-name error is returned if Name is already
// present. Save must still be called to persist the change.
func (m *Manager) Add(p Peer) error {
	if err := ValidatePeer(p); err != nil {
		return err
	}
	if _, ok := m.Find(p.Name); ok {
		return fmt.Errorf("peer %q already exists", p.Name)
	}
	m.peers = append(m.peers, p)
	return nil
}

// Remove drops the peer with the given name. Returns an error if the
// name is not present. Save must still be called to persist the
// change.
func (m *Manager) Remove(name string) error {
	for i, p := range m.peers {
		if p.Name == name {
			m.peers = append(m.peers[:i], m.peers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("peer %q not found", name)
}

// Save writes the in-memory peers list back to disk via an atomic
// rename. The file is created with mode 0640.
func (m *Manager) Save() error {
	body := encodePeers(m.peers)
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	return atomicWrite(m.path, []byte(body), 0o640)
}

// ValidatePeer checks that Name and Target are non-empty and Target is
// shaped like an rsync destination (`[user@]host:path`). The check is
// intentionally narrow — actual reachability is tested at push time.
func ValidatePeer(p Peer) error {
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("peer name is empty")
	}
	if strings.ContainsAny(p.Name, " \t\n\r\"\\") {
		return fmt.Errorf("peer name %q contains illegal whitespace or quote", p.Name)
	}
	return validateTarget(p.Target)
}

// validateTarget enforces the [user@]host:path shape on a peer
// target. The target is plaintext but ends up shelled out to rsync,
// so the parser refuses NUL, whitespace, and anything outside a
// conservative character set so a malicious peers file can't smuggle
// extra rsync arguments past us.
func validateTarget(target string) error {
	if target == "" {
		return errors.New("peer target is empty")
	}
	if strings.ContainsAny(target, "\x00 \t\n\r\"'\\;|&$`<>") {
		return fmt.Errorf(
			"peer target %q contains forbidden character", target,
		)
	}
	colon := strings.IndexByte(target, ':')
	if colon <= 0 || colon == len(target)-1 {
		return fmt.Errorf(
			"peer target %q must be [user@]host:path", target,
		)
	}
	if strings.HasPrefix(target, "-") {
		return fmt.Errorf("peer target %q must not start with '-'", target)
	}
	return nil
}

// atomicWrite mirrors internal/store/atomic.go but lives here so the
// peer package does not import the store package (which would be a
// cycle). The temp file lands in the same directory as path and is
// renamed atomically over the destination.
func atomicWrite(path string, content []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-peers-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
