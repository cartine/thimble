package quorum

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// Meta is the per-add metadata persisted by PrepareAdd into
// .pending-recipient-adds/meta.json. Verifiers re-read it to
// reconstruct the canonical message bytes and to know which
// operators were eligible at prepare time.
type Meta struct {
	// Version distinguishes future format changes. v1 is the only
	// shape known to this codebase.
	Version int `json:"version"`
	// App and Env scope the recipient add to one namespace.
	App string `json:"app"`
	Env string `json:"env"`
	// NewRecipient is the recipient string the maintainer wants to
	// add. Operators sign over this exact value.
	NewRecipient string `json:"new_recipient"`
	// BundleSHA is the on-disk bundle's SHA-256 at prepare time.
	// Recorded in the canonical message so a successful re-encrypt
	// (which changes the SHA) invalidates pending signatures.
	BundleSHA string `json:"bundle_sha"`
	// Nonce is a random hex string included in the canonical
	// message; protects against trivial structural collisions.
	Nonce string `json:"nonce"`
	// VerifierRecipient is the public recipient the maintainer's
	// identity will decrypt with. Operators re-encrypt their signed
	// message to this recipient so the verifier can decrypt and
	// confirm.
	VerifierRecipient string `json:"verifier_recipient"`
	// PolicyOperators is the snapshot of the operators table at
	// prepare time, captured to detect post-prepare policy edits.
	PolicyOperators []Operator `json:"policy_operators"`
	// QuorumM is the required signature count snapshotted at
	// prepare time.
	QuorumM int `json:"quorum_m"`
}

// SaveMeta writes m to MetaPath atomically (write-rename via the
// store's atomicWrite is awkward to import here, so we rename
// in-package). File mode is 0o600 to avoid leaking who is being added.
func SaveMeta(storeRoot string, m Meta) error {
	if err := os.MkdirAll(PendingDir(storeRoot), 0o700); err != nil {
		return fmt.Errorf("create pending dir: %w", err)
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	path := MetaPath(storeRoot)
	tmp, err := os.CreateTemp(PendingDir(storeRoot), "meta-*.json")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// LoadMeta reads and decodes the pending meta.json. A missing file
// returns (Meta{}, false, nil); any other error is fatal.
func LoadMeta(storeRoot string) (Meta, bool, error) {
	// #nosec G304 -- path is the configured store root joined with
	// fixed filenames; not user input at this layer.
	b, err := os.ReadFile(MetaPath(storeRoot))
	if errors.Is(err, os.ErrNotExist) {
		return Meta{}, false, nil
	}
	if err != nil {
		return Meta{}, false, fmt.Errorf("read meta: %w", err)
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, false, fmt.Errorf("decode meta: %w", err)
	}
	if m.Version != 1 {
		return Meta{}, false, fmt.Errorf("unsupported meta version %d", m.Version)
	}
	return m, true, nil
}

// ClearPending removes the entire pending directory and its
// contents. Called by the verifier after a successful add to start
// the next add from a clean slate.
func ClearPending(storeRoot string) error {
	return os.RemoveAll(PendingDir(storeRoot))
}
