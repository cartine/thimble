package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const manifestName = "thimble.json"

// Manifest is the persisted representation of the secrets store. It is
// plaintext by design: it lists applications, environments, recipients,
// and pointers to the encrypted bundles.
type Manifest struct {
	Version int                    `json:"version"`
	Apps    map[string]AppManifest `json:"apps"`
}

// AppManifest is the per-application slice of the Manifest.
type AppManifest struct {
	Environments map[string]EnvManifest `json:"environments"`
}

// EnvManifest describes one (application, environment) namespace: its
// bundle file, its recipients, and its timestamps.
type EnvManifest struct {
	Format     string   `json:"format"`
	File       string   `json:"file"`
	Recipients []string `json:"recipients"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// NamespaceView is a flattened (app, env) view of the manifest used by
// callers that want a sorted list of namespaces.
type NamespaceView struct {
	App        string
	Env        string
	Recipients int
	UpdatedAt  string
}

func (s *Store) loadManifest() (Manifest, error) {
	m := Manifest{Version: 1, Apps: map[string]AppManifest{}}
	path := filepath.Join(s.root, manifestName)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, err
	}
	if m.Apps == nil {
		m.Apps = map[string]AppManifest{}
	}
	return m, nil
}

func (s *Store) saveManifest(m Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return atomicWrite(filepath.Join(s.root, manifestName), b, 0o600)
}
