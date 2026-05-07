// K-37 origin-aware mutate path. The recipient ops (add/remove) keep
// using rewriteEnv from store.go because they do not change values
// and therefore do not change origins. The value-mutating ops
// (set/create/update/delete) and the K-37 rotate-after-removal flow
// route through rewriteEnvWithOrigins so origins are committed under
// the same exclusive flock as the manifest and bundle.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cartine/thimble/internal/dotenv"
)

// originsEditFn mutates the manifest entry, the dotenv values, and
// the origins map for one (app, env) namespace under the K-21
// optimistic-concurrency check. Returning an error aborts the commit
// and leaves disk untouched.
type originsEditFn func(
	meta *EnvManifest,
	values map[string]string,
	origins map[string]Origin,
) error

// rewriteEnvWithOrigins is the K-37 origin-aware sibling of
// rewriteEnv. It loads the manifest, the bundle, and the origins
// file, runs edit, then commits all three atomically. A missing
// origins file is treated as "no origins recorded yet" and is
// created on first commit (lazy migration for legacy namespaces).
func (s *Store) rewriteEnvWithOrigins(
	app, env string, edit originsEditFn,
) error {
	if err := ValidateName("app", app); err != nil {
		return err
	}
	if err := ValidateName("environment", env); err != nil {
		return err
	}
	m, meta, err := s.loadEnvForEdit(app, env)
	if err != nil {
		return err
	}
	loadedVersion := meta.Version
	priorSHA := meta.BundleSHA256
	plain, _, err := s.decrypt(app, env, meta)
	if err != nil {
		return err
	}
	values, err := dotenv.Parse(plain)
	if err != nil {
		return err
	}
	origins, err := s.loadOrigins(app, env)
	if err != nil {
		return err
	}
	if err := edit(&meta, values, origins); err != nil {
		return err
	}
	pruneOriginsToValues(origins, values)
	meta.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	meta.Version = loadedVersion + 1
	return s.commitEnvWithOrigins(commitInputs{
		app: app, env: env, m: m, meta: meta,
		loadedVersion: loadedVersion, priorSHA: priorSHA,
		plain: dotenv.Encode(values), origins: origins,
	})
}

// pruneOriginsToValues drops origin entries for keys that no longer
// exist in values. Keeps the origins file from accumulating dangling
// entries when keys are deleted.
func pruneOriginsToValues(origins map[string]Origin, values map[string]string) {
	for k := range origins {
		if _, ok := values[k]; !ok {
			delete(origins, k)
		}
	}
}

// commitInputs is a small struct so the commit helper can accept
// the bundled state without crossing the funlen limit on argument
// counts. Only used internally by rewriteEnvWithOrigins.
type commitInputs struct {
	app, env       string
	m              Manifest
	meta           EnvManifest
	loadedVersion  uint64
	priorSHA       string
	plain          string
	origins        map[string]Origin
}

// commitEnvWithOrigins is the origin-aware commit step. It mirrors
// commitEnv in store.go but additionally writes the origins file in
// the same critical section (so the manifest, bundle, and origins
// file land or roll back as a unit).
func (s *Store) commitEnvWithOrigins(in commitInputs) error {
	lock, err := lockExclusive(s.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	disk, err := s.loadManifest()
	if err != nil {
		return err
	}
	if cur, ok := envFrom(disk, in.app, in.env); ok && cur.Version != in.loadedVersion {
		return fmt.Errorf(
			"another writer changed %s/%s; rerun", in.app, in.env,
		)
	}
	priorBundle, err := s.snapshotBundleBytes(in.app, in.env, in.meta)
	if err != nil {
		return err
	}
	priorOrigins, err := s.originsSnapshotBytes(in.app, in.env)
	if err != nil {
		return err
	}
	if err := s.encryptAndWrite(&in.meta, in.plain); err != nil {
		return err
	}
	if err := s.saveOriginsWithHook(in.app, in.env, in.origins); err != nil {
		s.rollbackBundle(in.app, in.env, in.meta, priorBundle)
		s.restoreOriginsBytesIgnoreErr(in.app, in.env, priorOrigins)
		return fmt.Errorf("save origins: %w", err)
	}
	if in.priorSHA == "" && in.meta.BundleSHA256 != "" {
		s.notify(
			"bundle %s/%s: BundleSHA256 was empty; populated to %s",
			in.app, in.env, in.meta.BundleSHA256,
		)
	}
	mergeInto(&in.m, disk, in.app, in.env, in.meta)
	if err := s.saveManifest(in.m); err != nil {
		s.rollbackBundle(in.app, in.env, in.meta, priorBundle)
		s.restoreOriginsBytesIgnoreErr(in.app, in.env, priorOrigins)
		return fmt.Errorf("save manifest: %w", err)
	}
	return nil
}

// snapshotBundleBytes reads the current ciphertext bytes for the
// (app, env) bundle. Returns nil on missing file (e.g. very first
// write). Used to roll back on partial-failure.
func (s *Store) snapshotBundleBytes(
	app, env string, meta EnvManifest,
) ([]byte, error) {
	_ = app
	_ = env
	bundlePath := filepath.Join(s.root, filepath.FromSlash(meta.File))
	// #nosec G304 -- bundlePath is computed from validated app/env
	// names within the store root.
	b, err := os.ReadFile(bundlePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("snapshot bundle: %w", err)
	}
	return b, nil
}

// rollbackBundle restores the prior ciphertext bytes for the
// (app, env) bundle. Errors are reported via the notice writer so
// the user sees them but the original error is what's returned.
func (s *Store) rollbackBundle(
	app, env string, meta EnvManifest, prior []byte,
) {
	bundlePath := filepath.Join(s.root, filepath.FromSlash(meta.File))
	if prior == nil {
		if err := os.Remove(bundlePath); err != nil && !os.IsNotExist(err) {
			s.notify(
				"warning: rollback bundle %s/%s: %v", app, env, err,
			)
		}
		return
	}
	if err := atomicWrite(bundlePath, prior, 0o600); err != nil {
		s.notify(
			"warning: rollback bundle %s/%s: %v", app, env, err,
		)
	}
}

// restoreOriginsBytesIgnoreErr is the rollback wrapper for
// restoreOriginsBytes. Errors are surfaced through the notice writer
// so they don't mask the original commit failure.
func (s *Store) restoreOriginsBytesIgnoreErr(
	app, env string, prior []byte,
) {
	if err := s.restoreOriginsBytes(app, env, prior); err != nil {
		s.notify(
			"warning: rollback origins %s/%s: %v", app, env, err,
		)
	}
}
