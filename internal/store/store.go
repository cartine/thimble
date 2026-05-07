// Package store owns the on-disk secrets layout: the plaintext
// thimble.json manifest and the per-namespace encrypted bundles. It is
// the only package that calls internal/age and the only one that
// writes to the secrets directory; plaintext only lives in memory for
// the duration of a CRUD call.
package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/dotenv"
)

// Store wraps a secrets directory plus an *age.Tool. It is the only
// type that reads or writes the secrets/ tree; callers that need a
// view (e.g. the web UI, render command) ask Store for it.
type Store struct {
	root    string
	age     *age.Tool
	now     func() time.Time
	baseCtx context.Context
	notice  io.Writer
	audit   *auditState
}

// New returns a Store rooted at root that decrypts with identity. The
// age binary name defaults to "age" and is resolved through PATH on
// first use; callers that need to pin a specific binary or SHA-256
// should call NewWithAge.
func New(root, identity string) *Store {
	return &Store{
		root:    root,
		age:     age.New("age", identity),
		now:     time.Now,
		baseCtx: context.Background(),
	}
}

// NewWithAge returns a Store that uses tool for all encrypt/decrypt
// operations. Construct tool via age.New / age.Resolve so the binary
// path is pinned at startup (K-18).
func NewWithAge(root string, tool *age.Tool) *Store {
	return &Store{root: root, age: tool, now: time.Now, baseCtx: context.Background()}
}

// SetContext installs a base context that the Store passes to every
// age subprocess invocation. The CLI wires SIGINT-cancelable contexts
// through here so Ctrl-C interrupts a stuck age call cleanly (K-26).
func (s *Store) SetContext(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.baseCtx = ctx
}

// Root returns the directory the Store reads and writes.
func (s *Store) Root() string { return s.root }

// SetAge replaces the underlying age tool. Used by tests to swap in a
// fake age binary path.
func (s *Store) SetAge(t *age.Tool) { s.age = t }

// SetClock replaces the time source. Used by tests to make timestamps
// deterministic.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

// SetNoticeWriter installs a writer for one-time advisory notices
// (e.g. K-22's "BundleSHA256 was empty; populated to ..." upgrade
// line). The CLI wires stderr through here. nil silences notices.
func (s *Store) SetNoticeWriter(w io.Writer) { s.notice = w }

// notify writes an advisory line to the notice writer if one is set.
func (s *Store) notify(format string, args ...interface{}) {
	if s.notice == nil {
		return
	}
	fmt.Fprintf(s.notice, format+"\n", args...)
}

// Init creates a new namespace with an initial recipient list. It
// fails if the namespace already exists. Held under an exclusive
// flock so two operators racing Init cannot trample each other.
func (s *Store) Init(app, env string, recipients []string) error {
	if err := ValidateName("app", app); err != nil {
		return err
	}
	if err := ValidateName("environment", env); err != nil {
		return err
	}
	cleaned, err := CleanRecipients(recipients)
	if err != nil {
		return err
	}
	lock, err := lockExclusive(s.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	m, err := s.loadManifest()
	if err != nil {
		return err
	}
	if _, ok := m.Apps[app]; !ok {
		m.Apps[app] = AppManifest{Environments: map[string]EnvManifest{}}
	}
	if _, ok := m.Apps[app].Environments[env]; ok {
		return fmt.Errorf("%s/%s already exists", app, env)
	}
	now := s.now().UTC().Format(time.RFC3339)
	envMeta := EnvManifest{
		Format:     "dotenv",
		File:       filepath.ToSlash(filepath.Join(app, env+".env.age")),
		Recipients: sortedUnique(cleaned),
		CreatedAt:  now,
		UpdatedAt:  now,
		Version:    1,
	}
	if err := s.encryptAndWrite(&envMeta, ""); err != nil {
		return err
	}
	m.Apps[app].Environments[env] = envMeta
	if err := s.saveManifest(m); err != nil {
		return err
	}
	s.recordEvent(auditOpInit, app, env, "")
	return nil
}

// AddRecipient grants recipient access to (app, env) and re-encrypts
// the bundle to the updated recipient list. The audit log records
// the recipient by thumbprint, never the full string (K-27).
func (s *Store) AddRecipient(app, env, recipient string) error {
	cleaned, err := CleanRecipient(recipient)
	if err != nil {
		return err
	}
	err = s.rewriteEnv(app, env, func(meta *EnvManifest, _ map[string]string) error {
		meta.Recipients = sortedUnique(append(meta.Recipients, cleaned))
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpRecipientAdd, app, env, recipientThumbprint(cleaned))
	return nil
}

// RemoveRecipient drops recipient from (app, env) and re-encrypts.
// Refuses to remove the last recipient. The lookup is normalized so
// trailing newlines are tolerated. Audited by recipient thumbprint.
func (s *Store) RemoveRecipient(app, env, recipient string) error {
	cleaned, err := CleanRecipient(recipient)
	if err != nil {
		return err
	}
	err = s.rewriteEnv(app, env, func(meta *EnvManifest, _ map[string]string) error {
		next := meta.Recipients[:0]
		for _, existing := range meta.Recipients {
			if existing != cleaned {
				next = append(next, existing)
			}
		}
		if len(next) == len(meta.Recipients) {
			return fmt.Errorf("recipient not found")
		}
		if len(next) == 0 {
			return errors.New("cannot remove the last recipient")
		}
		meta.Recipients = sortedUnique(next)
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpRecipientRemove, app, env, recipientThumbprint(cleaned))
	return nil
}

// CreateSecret adds key to (app, env), failing if it already exists.
func (s *Store) CreateSecret(app, env, key, value string) error {
	err := s.rewriteEnv(app, env, func(_ *EnvManifest, values map[string]string) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; ok {
			return fmt.Errorf("%s already exists; use update or set", key)
		}
		values[key] = value
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpCreate, app, env, key)
	return nil
}

// UpdateSecret overwrites an existing key in (app, env), failing if
// the key is missing.
func (s *Store) UpdateSecret(app, env, key, value string) error {
	err := s.rewriteEnv(app, env, func(_ *EnvManifest, values map[string]string) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; !ok {
			return fmt.Errorf("%s does not exist; use create or set", key)
		}
		values[key] = value
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpUpdate, app, env, key)
	return nil
}

// SetSecret creates or updates key in (app, env). Idempotent.
func (s *Store) SetSecret(app, env, key, value string) error {
	err := s.rewriteEnv(app, env, func(_ *EnvManifest, values map[string]string) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		values[key] = value
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpSet, app, env, key)
	return nil
}

// DeleteSecret removes key from (app, env), failing if missing.
func (s *Store) DeleteSecret(app, env, key string) error {
	err := s.rewriteEnv(app, env, func(_ *EnvManifest, values map[string]string) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; !ok {
			return fmt.Errorf("%s does not exist", key)
		}
		delete(values, key)
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpDelete, app, env, key)
	return nil
}

// ListSecrets returns the sorted keys present in (app, env). It never
// returns values.
func (s *Store) ListSecrets(app, env string) ([]string, error) {
	values, _, err := s.ReadEnv(app, env)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// Render decrypts (app, env) and returns the dotenv-encoded plaintext.
func (s *Store) Render(app, env string) (string, error) {
	values, _, err := s.ReadEnv(app, env)
	if err != nil {
		return "", err
	}
	return dotenv.Encode(values), nil
}

// ListNamespaces returns a flattened, sorted view of every (app, env)
// pair recorded in the manifest. Held under a shared flock so a
// concurrent writer cannot tear the read.
func (s *Store) ListNamespaces() ([]NamespaceView, error) {
	lock, err := lockShared(s.root)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	m, err := s.loadManifest()
	if err != nil {
		return nil, err
	}
	var views []NamespaceView
	for app, appMeta := range m.Apps {
		for env, envMeta := range appMeta.Environments {
			views = append(views, NamespaceView{
				App:        app,
				Env:        env,
				Recipients: len(envMeta.Recipients),
				UpdatedAt:  envMeta.UpdatedAt,
			})
		}
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].App == views[j].App {
			return views[i].Env < views[j].Env
		}
		return views[i].App < views[j].App
	})
	return views, nil
}

// Find returns the EnvManifest for (app, env), or an error if the
// namespace is not initialized. Held under a shared flock.
func (s *Store) Find(app, env string) (EnvManifest, error) {
	if err := ValidateName("app", app); err != nil {
		return EnvManifest{}, err
	}
	if err := ValidateName("environment", env); err != nil {
		return EnvManifest{}, err
	}
	lock, err := lockShared(s.root)
	if err != nil {
		return EnvManifest{}, err
	}
	defer lock.Close()
	m, err := s.loadManifest()
	if err != nil {
		return EnvManifest{}, err
	}
	appMeta, ok := m.Apps[app]
	if !ok {
		return EnvManifest{}, fmt.Errorf("%s/%s is not initialized", app, env)
	}
	meta, ok := appMeta.Environments[env]
	if !ok {
		return EnvManifest{}, fmt.Errorf("%s/%s is not initialized", app, env)
	}
	return meta, nil
}

// ReadEnv decrypts (app, env) and returns its parsed key/value map
// alongside the manifest record. The K-22 SHA verification fires on
// every decrypt so a tampered bundle is rejected before age runs.
func (s *Store) ReadEnv(app, env string) (map[string]string, EnvManifest, error) {
	meta, err := s.Find(app, env)
	if err != nil {
		return nil, EnvManifest{}, err
	}
	plain, _, err := s.decrypt(app, env, meta)
	if err != nil {
		return nil, EnvManifest{}, err
	}
	values, err := dotenv.Parse(plain)
	if err != nil {
		return nil, EnvManifest{}, err
	}
	return values, meta, nil
}

// rewriteEnv applies edit to the (app, env) namespace under K-21
// optimistic concurrency. It loads the manifest unlocked, runs edit,
// then re-acquires an exclusive flock to verify the on-disk Version
// has not changed before encrypting and saving. K-22: a missing
// stored BundleSHA256 (older manifest) is upgraded silently here;
// commitEnv emits the upgrade notice once the new SHA lands.
func (s *Store) rewriteEnv(
	app, env string,
	edit func(*EnvManifest, map[string]string) error,
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
	if err := edit(&meta, values); err != nil {
		return err
	}
	meta.UpdatedAt = s.now().UTC().Format(time.RFC3339)
	meta.Version = loadedVersion + 1
	return s.commitEnv(
		app, env, m, meta, loadedVersion, priorSHA, dotenv.Encode(values),
	)
}

// loadEnvForEdit reads the manifest under a shared lock and returns
// the manifest and the env's manifest entry, or an "is not
// initialized" error if missing.
func (s *Store) loadEnvForEdit(app, env string) (Manifest, EnvManifest, error) {
	lock, err := lockShared(s.root)
	if err != nil {
		return Manifest{}, EnvManifest{}, err
	}
	defer lock.Close()
	m, err := s.loadManifest()
	if err != nil {
		return Manifest{}, EnvManifest{}, err
	}
	appMeta, ok := m.Apps[app]
	if !ok {
		return Manifest{}, EnvManifest{}, fmt.Errorf("%s/%s is not initialized", app, env)
	}
	meta, ok := appMeta.Environments[env]
	if !ok {
		return Manifest{}, EnvManifest{}, fmt.Errorf("%s/%s is not initialized", app, env)
	}
	return m, meta, nil
}

// commitEnv re-reads the manifest under an exclusive lock, refuses if
// the on-disk Version has advanced past loadedVersion, and otherwise
// writes the new ciphertext and bumped manifest atomically. K-22: if
// priorSHA was empty (older manifest), the freshly populated
// BundleSHA256 is announced via the notice writer once the write
// succeeds.
func (s *Store) commitEnv(
	app, env string,
	m Manifest, meta EnvManifest, loadedVersion uint64,
	priorSHA, plain string,
) error {
	lock, err := lockExclusive(s.root)
	if err != nil {
		return err
	}
	defer lock.Close()
	disk, err := s.loadManifest()
	if err != nil {
		return err
	}
	if cur, ok := envFrom(disk, app, env); ok && cur.Version != loadedVersion {
		return fmt.Errorf("another writer changed %s/%s; rerun", app, env)
	}
	if err := s.encryptAndWrite(&meta, plain); err != nil {
		return err
	}
	if priorSHA == "" && meta.BundleSHA256 != "" {
		s.notify(
			"bundle %s/%s: BundleSHA256 was empty; populated to %s",
			app, env, meta.BundleSHA256,
		)
	}
	mergeInto(&m, disk, app, env, meta)
	return s.saveManifest(m)
}

// envFrom returns the EnvManifest for (app, env) from m, if present.
func envFrom(m Manifest, app, env string) (EnvManifest, bool) {
	appMeta, ok := m.Apps[app]
	if !ok {
		return EnvManifest{}, false
	}
	meta, ok := appMeta.Environments[env]
	return meta, ok
}

// mergeInto installs the edited (app, env) entry into m using the
// freshly read disk manifest as the base, so a concurrent edit to a
// different namespace is not lost.
func mergeInto(m *Manifest, disk Manifest, app, env string, meta EnvManifest) {
	*m = disk
	if m.Apps == nil {
		m.Apps = map[string]AppManifest{}
	}
	appMeta, ok := m.Apps[app]
	if !ok {
		appMeta = AppManifest{Environments: map[string]EnvManifest{}}
	}
	if appMeta.Environments == nil {
		appMeta.Environments = map[string]EnvManifest{}
	}
	appMeta.Environments[env] = meta
	m.Apps[app] = appMeta
}

