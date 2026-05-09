package store

import "path/filepath"

// LoadManifest returns a copy of the on-disk manifest, taken under
// a shared flock. Diagnostic callers (K-29 thimble doctor) use this
// to walk every namespace without going through ReadEnv (which
// decrypts and is therefore loud and slow).
func (s *Store) LoadManifest() (Manifest, error) {
	lock, err := lockShared(s.root)
	if err != nil {
		return Manifest{}, err
	}
	defer lock.Close()
	return s.loadManifest()
}

// BundlePath returns the absolute path Thimble uses for (app, env)'s
// ciphertext file. Useful for diagnostics that want to stat the
// bundle without reading the manifest themselves.
func (s *Store) BundlePath(meta EnvManifest) string {
	return filepath.Join(s.root, filepath.FromSlash(meta.File))
}
