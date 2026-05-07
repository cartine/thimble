// K-37 recipient remove + rotate flow. RemoveRecipientWithRotation
// extends the legacy RemoveRecipient by walking the namespace's
// origins map after the recipient is dropped: every key whose origin
// is OriginProvision is regenerated in place. Operator-supplied keys
// (OriginSet, OriginAndSet, or unknown origin) are surfaced in the
// outcome so the caller can print "manual rotate needed" lines.
package store

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
)

// RotateOptions controls the K-37 rotate-after-removal flow. Both
// fields default to false, in which case RemoveRecipientWithRotation
// behaves exactly like the legacy RemoveRecipient (no rotation, no
// extra audit events).
type RotateOptions struct {
	// Rotate enables value rotation for every OriginProvision key
	// after the recipient is removed. Operator-supplied keys are
	// reported through RotateOutcome.NeedsAttention.
	Rotate bool

	// RandomsOnly is the --rotate-randoms-only mode: rotate the
	// provisioned keys silently and do not surface NeedsAttention.
	// Only meaningful when Rotate is true.
	RandomsOnly bool
}

// RotateOutcome reports the K-37 outcome of
// RemoveRecipientWithRotation. Rotated lists every key whose value
// was regenerated; NeedsAttention lists every operator-supplied key
// that the caller should surface to the user. The slices are sorted
// for deterministic output.
type RotateOutcome struct {
	Rotated        []string
	NeedsAttention []NeedsAttentionEntry
}

// NeedsAttentionEntry pairs a key with its origin so the CLI can
// produce a useful "manual rotate needed: KEY (origin: ...)" line.
type NeedsAttentionEntry struct {
	Key    string
	Origin Origin
}

// RemoveRecipientWithRotation removes recipient from (app, env). If
// opts.Rotate is true, every OriginProvision key in the namespace is
// regenerated atomically alongside the recipient removal. The whole
// flow runs under one rewriteEnvWithOrigins critical section so the
// manifest, bundle, and origins file land or roll back as a unit.
func (s *Store) RemoveRecipientWithRotation(
	app, env, recipient string, opts RotateOptions,
) (RotateOutcome, error) {
	cleaned, err := CleanRecipient(recipient)
	if err != nil {
		return RotateOutcome{}, err
	}
	var outcome RotateOutcome
	err = s.rewriteEnvWithOrigins(app, env, func(
		meta *EnvManifest, values map[string]string,
		origins map[string]Origin,
	) error {
		if err := dropRecipient(meta, cleaned); err != nil {
			return err
		}
		if !opts.Rotate {
			return nil
		}
		out, err := rotateProvisionedKeys(values, origins, opts.RandomsOnly)
		if err != nil {
			return err
		}
		outcome = out
		return nil
	})
	if err != nil {
		return RotateOutcome{}, err
	}
	s.recordEvent(auditOpRecipientRemove, app, env, recipientThumbprint(cleaned))
	for _, key := range outcome.Rotated {
		s.recordEvent(auditOpUpdate, app, env, key)
	}
	return outcome, nil
}

// dropRecipient mutates meta to remove cleaned from the recipient
// list. It refuses if cleaned is not present or if removing would
// leave the namespace with zero recipients (mirrors the legacy
// RemoveRecipient invariant).
func dropRecipient(meta *EnvManifest, cleaned string) error {
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
}

// rotateProvisionedKeys walks values+origins and regenerates every
// OriginProvision value in place. randomsOnly suppresses the
// NeedsAttention output so callers using `--rotate-randoms-only` get
// only the rotation summary.
func rotateProvisionedKeys(
	values map[string]string, origins map[string]Origin,
	randomsOnly bool,
) (RotateOutcome, error) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out RotateOutcome
	for _, k := range keys {
		origin := originOrDefault(origins, k)
		if origin == OriginProvision {
			fresh, err := generateProvisionValue()
			if err != nil {
				return RotateOutcome{}, err
			}
			values[k] = fresh
			origins[k] = OriginProvision
			out.Rotated = append(out.Rotated, k)
			continue
		}
		if randomsOnly {
			continue
		}
		out.NeedsAttention = append(out.NeedsAttention,
			NeedsAttentionEntry{Key: k, Origin: origin},
		)
	}
	return out, nil
}

// generateProvisionValue is the value-generation logic shared with
// `thimble provision`. 32 random bytes encoded as base64-url
// without padding, matching the default `provision` output. Kept
// here (rather than imported from internal/cli) to avoid a layer
// inversion: the store package must not depend on the CLI.
func generateProvisionValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
