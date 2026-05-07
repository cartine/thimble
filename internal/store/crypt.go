package store

import (
	"context"
	"fmt"
	"path/filepath"
)

// encryptAndWrite encrypts plain to meta's recipients, writes the
// resulting ciphertext to meta.File atomically, and stores the
// ciphertext's SHA-256 in meta.BundleSHA256. Callers must persist the
// updated meta into the manifest after a successful return so the
// next decrypt's verification matches what was written (K-22).
func (s *Store) encryptAndWrite(meta *EnvManifest, plain string) error {
	if err := ValidateRecipients(meta.Recipients); err != nil {
		return err
	}
	cipher, err := s.age.Encrypt(s.context(), meta.Recipients, plain)
	if err != nil {
		return err
	}
	bundlePath := filepath.Join(s.root, filepath.FromSlash(meta.File))
	if err := atomicWrite(bundlePath, cipher, 0o600); err != nil {
		return err
	}
	meta.BundleSHA256 = bytesSHA256(cipher)
	return nil
}

// decrypt verifies the bundle's SHA-256 against meta.BundleSHA256
// (K-22) before shelling out to age. A non-empty SHA mismatch is a
// hard error; an empty stored SHA falls through to age and the
// recomputed value is returned for the caller to populate. The
// caller passes app and env so the eventual error or upgrade note
// names the namespace.
func (s *Store) decrypt(app, env string, meta EnvManifest) (string, string, error) {
	bundlePath := filepath.Join(s.root, filepath.FromSlash(meta.File))
	actual, err := fileSHA256(bundlePath)
	if err != nil {
		return "", "", fmt.Errorf("hashing %s/%s bundle: %w", app, env, err)
	}
	if meta.BundleSHA256 != "" && meta.BundleSHA256 != actual {
		return "", actual, fmt.Errorf(
			"bundle %s/%s SHA-256 mismatch (manifest=%s file=%s); "+
				"refusing to decrypt",
			app, env, meta.BundleSHA256, actual,
		)
	}
	plain, err := s.age.Decrypt(s.context(), bundlePath)
	if err != nil {
		return "", actual, err
	}
	return plain, actual, nil
}

// context returns the base context for an age invocation, falling
// back to context.Background() if SetContext was never called.
func (s *Store) context() context.Context {
	if s.baseCtx == nil {
		return context.Background()
	}
	return s.baseCtx
}
