// K-37 origin-aware secret CRUD wrappers. The non-origin entry
// points (CreateSecret, UpdateSecret, SetSecret) defined in store.go
// route through these so an explicit origin is recorded for every
// mutation. Existing callers that don't care about origin keep the
// short signatures and get OriginSet by default.
package store

import (
	"fmt"

	"github.com/cartine/thimble/internal/dotenv"
)

// CreateSecretWithOrigin is the K-37 origin-aware variant of
// CreateSecret. The origin label is stamped onto the namespace's
// origins file under the same exclusive flock as the manifest and
// bundle, so a partial write never leaves the three out of sync.
func (s *Store) CreateSecretWithOrigin(
	app, env, key, value string, origin Origin,
) error {
	err := s.rewriteEnvWithOrigins(app, env, func(
		_ *EnvManifest, values map[string]string,
		origins map[string]Origin,
	) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; ok {
			return fmt.Errorf("%s already exists; use update or set", key)
		}
		values[key] = value
		origins[key] = origin
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpCreate, app, env, key)
	return nil
}

// UpdateSecretWithOrigin is the K-37 origin-aware variant of
// UpdateSecret. Behaves like UpdateSecret on values but also stamps
// origin on the origins file.
func (s *Store) UpdateSecretWithOrigin(
	app, env, key, value string, origin Origin,
) error {
	err := s.rewriteEnvWithOrigins(app, env, func(
		_ *EnvManifest, values map[string]string,
		origins map[string]Origin,
	) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; !ok {
			return fmt.Errorf("%s does not exist; use create or set", key)
		}
		values[key] = value
		origins[key] = origin
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpUpdate, app, env, key)
	return nil
}

// SetSecretWithOrigin is the K-37 origin-aware variant of SetSecret.
// Idempotent on values; idempotent on origins (the latest write wins).
func (s *Store) SetSecretWithOrigin(
	app, env, key, value string, origin Origin,
) error {
	err := s.rewriteEnvWithOrigins(app, env, func(
		_ *EnvManifest, values map[string]string,
		origins map[string]Origin,
	) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		values[key] = value
		origins[key] = origin
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpSet, app, env, key)
	return nil
}

// DeleteSecretWithOrigin is the K-37 origin-aware variant of
// DeleteSecret. Removes both the value and its origin entry under
// the same exclusive flock so the origins file does not accumulate
// stale entries.
func (s *Store) DeleteSecretWithOrigin(app, env, key string) error {
	err := s.rewriteEnvWithOrigins(app, env, func(
		_ *EnvManifest, values map[string]string,
		origins map[string]Origin,
	) error {
		if err := dotenv.ValidateKey(key); err != nil {
			return err
		}
		if _, ok := values[key]; !ok {
			return fmt.Errorf("%s does not exist", key)
		}
		delete(values, key)
		delete(origins, key)
		return nil
	})
	if err != nil {
		return err
	}
	s.recordEvent(auditOpDelete, app, env, key)
	return nil
}
