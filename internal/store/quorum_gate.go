// K-36 quorum gate: when secrets/recipients.signed.toml is present,
// recipient additions require M-of-N signatures from the listed
// operators before the bundle is re-encrypted. This file owns the
// integration between Store.AddRecipient and the internal/quorum
// package; it preserves the existing single-call semantics when the
// policy is absent so behavior is opt-in.
package store

import (
	"errors"
	"fmt"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/quorum"
)

// AddRecipientOptions controls the K-36 add modes. Bootstrap=true
// asks the gate to bypass the quorum check; it succeeds only when
// the namespace currently has fewer than 2 recipients (initial
// setup). The default zero value is "regular add"; Store routes it
// to the quorum gate when the policy file exists, otherwise to the
// pre-K-36 direct-add path.
type AddRecipientOptions struct {
	Bootstrap bool
}

// AddOutcome reports the path taken by AddRecipientV2. Stage is one
// of "added", "prepared". On "prepared" the maintainer should
// distribute the pending directory and have operators sign-add;
// re-running AddRecipientV2 finalizes the gate.
type AddOutcome struct {
	Stage           string
	SignerThumbs    []string
	SignerNames     []string
	OperatorsCount  int
	QuorumM         int
	NewRecipient    string
	PolicyOperators []PolicyOperatorView
}

// PolicyOperatorView is a CLI-friendly mirror of quorum.Operator
// exposed by the store package so callers do not need to import
// internal/quorum directly.
type PolicyOperatorView struct {
	Name      string
	Recipient string
}

// toOperatorViews converts a slice of quorum.Operator into the
// CLI-facing view type. Kept short so call sites remain readable.
func toOperatorViews(ops []quorum.Operator) []PolicyOperatorView {
	out := make([]PolicyOperatorView, len(ops))
	for i, op := range ops {
		out[i] = PolicyOperatorView{Name: op.Name, Recipient: op.Recipient}
	}
	return out
}

// AddRecipientV2 is the K-36 entry point for adding a recipient. The
// pre-K-36 AddRecipient is preserved as the bootstrap-equivalent path
// for callers that have not adopted the new options struct; it now
// routes through AddRecipientV2 with Bootstrap=true so audit fields
// are recorded consistently.
func (s *Store) AddRecipientV2(
	app, env, recipient string, opts AddRecipientOptions,
) (AddOutcome, error) {
	cleaned, err := CleanRecipient(recipient)
	if err != nil {
		return AddOutcome{}, err
	}
	policy, present, err := quorum.LoadPolicy(quorum.PolicyPath(s.root))
	if err != nil {
		return AddOutcome{}, err
	}
	if !present {
		return s.legacyAdd(app, env, cleaned, opts)
	}
	if err := validatePolicyAgainstStore(policy); err != nil {
		return AddOutcome{}, err
	}
	if opts.Bootstrap {
		return s.bootstrapAdd(app, env, cleaned, policy)
	}
	return s.quorumAdd(app, env, cleaned, policy)
}

// legacyAdd is the pre-K-36 path. Behavior is identical to the
// existing AddRecipient: re-encrypt the bundle with the new
// recipient and record the audit event without signers.
func (s *Store) legacyAdd(
	app, env, cleaned string, opts AddRecipientOptions,
) (AddOutcome, error) {
	if err := s.applyAdd(app, env, cleaned); err != nil {
		return AddOutcome{}, err
	}
	s.recordEventWithSigners(
		auditOpRecipientAdd, app, env,
		recipientThumbprint(cleaned), nil, opts.Bootstrap,
	)
	return AddOutcome{Stage: "added", NewRecipient: cleaned}, nil
}

// bootstrapAdd is the K-36 chicken-and-egg path. Allowed only when
// the namespace has 0 or 1 recipients; the quorum gate is bypassed
// because no policy is meaningfully enforceable yet. Audit records
// bootstrap=true so reviewers see which adds skipped the gate.
func (s *Store) bootstrapAdd(
	app, env, cleaned string, _ quorum.Policy,
) (AddOutcome, error) {
	count, err := s.recipientCount(app, env)
	if err != nil {
		return AddOutcome{}, err
	}
	if count >= 2 {
		return AddOutcome{}, fmt.Errorf(
			"--bootstrap rejected: %s/%s has %d recipients; "+
				"use the quorum flow (sign-add then recipient add)",
			app, env, count,
		)
	}
	if err := s.applyAdd(app, env, cleaned); err != nil {
		return AddOutcome{}, err
	}
	s.recordEventWithSigners(
		auditOpRecipientAdd, app, env,
		recipientThumbprint(cleaned), nil, true,
	)
	return AddOutcome{Stage: "added", NewRecipient: cleaned}, nil
}

// quorumAdd is the gated path. If no pending challenge set exists,
// it prepares one and returns Stage="prepared". If a pending set
// does exist for this same (app, env, recipient), it verifies
// signatures and, if M satisfied, commits the add and clears the
// pending dir.
func (s *Store) quorumAdd(
	app, env, cleaned string, policy quorum.Policy,
) (AddOutcome, error) {
	meta, present, err := quorum.LoadMeta(s.root)
	if err != nil {
		return AddOutcome{}, err
	}
	if !present {
		return s.preparePhase(app, env, cleaned, policy)
	}
	if err := matchesPending(meta, app, env, cleaned); err != nil {
		return AddOutcome{}, err
	}
	return s.verifyPhase(app, env, cleaned, policy)
}

// preparePhase generates the per-operator challenge files and
// returns Stage="prepared". The maintainer is expected to share the
// resulting pending directory with operators (or commit it to a
// short-lived branch), have them run sign-add, then re-run this
// command to finalize.
func (s *Store) preparePhase(
	app, env, cleaned string, policy quorum.Policy,
) (AddOutcome, error) {
	meta, err := s.Find(app, env)
	if err != nil {
		return AddOutcome{}, err
	}
	verifier := s.operatorRecipient()
	if verifier == "" {
		return AddOutcome{}, errors.New(
			"prepare requires THIMBLE_AGE_IDENTITY to be set " +
				"(verifier identity must be derivable for sign-add to address)",
		)
	}
	in := quorum.PrepareInputs{
		StoreRoot:         s.root,
		App:               app,
		Env:               env,
		NewRecipient:      cleaned,
		BundleSHA:         meta.BundleSHA256,
		Policy:            policy,
		VerifierRecipient: verifier,
		Age:               s.age,
	}
	if err := quorum.PrepareAdd(s.context(), in); err != nil {
		return AddOutcome{}, err
	}
	return AddOutcome{
		Stage:           "prepared",
		NewRecipient:    cleaned,
		QuorumM:         policy.M,
		OperatorsCount:  len(policy.Operators),
		PolicyOperators: toOperatorViews(policy.Operators),
	}, nil
}

// verifyPhase validates the collected signatures against the
// policy, requires ≥M valid distinct operators, then commits the
// add and clears the pending directory.
func (s *Store) verifyPhase(
	app, env, cleaned string, policy quorum.Policy,
) (AddOutcome, error) {
	envMeta, err := s.Find(app, env)
	if err != nil {
		return AddOutcome{}, err
	}
	verifyResult, err := quorum.Verify(s.context(), quorum.VerifyInputs{
		StoreRoot:    s.root,
		App:          app,
		Env:          env,
		NewRecipient: cleaned,
		BundleSHA:    envMeta.BundleSHA256,
		Policy:       policy,
		Age:          s.age,
	})
	if err != nil {
		return AddOutcome{}, err
	}
	if err := s.applyAdd(app, env, cleaned); err != nil {
		return AddOutcome{}, err
	}
	s.recordEventWithSigners(
		auditOpRecipientAdd, app, env,
		recipientThumbprint(cleaned),
		verifyResult.SignerThumbs, false,
	)
	if err := quorum.ClearPending(s.root); err != nil {
		s.notify(
			"warning: failed to clear pending dir after add: %v", err,
		)
	}
	return AddOutcome{
		Stage:        "added",
		NewRecipient: cleaned,
		SignerThumbs: verifyResult.SignerThumbs,
		SignerNames:  verifyResult.SignerNames,
		QuorumM:      policy.M,
	}, nil
}

// applyAdd is the post-gate write step. It mutates the manifest and
// re-encrypts the bundle; identical to the historical AddRecipient
// body without the audit event (the caller emits the richer one).
func (s *Store) applyAdd(app, env, cleaned string) error {
	return s.rewriteEnv(app, env, func(meta *EnvManifest, _ map[string]string) error {
		meta.Recipients = sortedUnique(append(meta.Recipients, cleaned))
		return nil
	})
}

// matchesPending refuses if meta.json describes a different add
// than the one the maintainer is finalizing. Catches operator
// fat-finger ("you prepared X but typed Y") before any IO.
func matchesPending(meta quorum.Meta, app, env, cleaned string) error {
	if meta.NewRecipient != cleaned {
		return fmt.Errorf(
			"pending add is for %s; cannot finalize against %s "+
				"(re-prepare or remove %s)",
			meta.NewRecipient, cleaned, quorum.MetaPath(""),
		)
	}
	if meta.App != app || meta.Env != env {
		return fmt.Errorf(
			"pending add is for %s/%s; cannot finalize against %s/%s",
			meta.App, meta.Env, app, env,
		)
	}
	return nil
}

// validatePolicyAgainstStore enforces that every recipient string
// in the policy is itself a valid Thimble recipient. The quorum
// package doesn't import store to avoid a cycle, so we re-check here.
func validatePolicyAgainstStore(p quorum.Policy) error {
	for _, op := range p.Operators {
		if _, err := CleanRecipient(op.Recipient); err != nil {
			return fmt.Errorf(
				"policy operator %q: %w", op.Name, err,
			)
		}
	}
	return nil
}

// recipientCount returns how many recipients (app, env) currently
// has, or 0 if the namespace does not exist (so bootstrap can be
// allowed before init is run for completeness, even though Init is
// the typical first step).
func (s *Store) recipientCount(app, env string) (int, error) {
	meta, err := s.Find(app, env)
	if err != nil {
		return 0, err
	}
	return len(meta.Recipients), nil
}

// operatorRecipient returns the maintainer's public recipient
// string, derived from their identity file via the K-27 helper.
// Used at prepare time as the verifier address operators encrypt
// their signatures to.
func (s *Store) operatorRecipient() string {
	if s.age == nil {
		return ""
	}
	identity := s.age.Identity()
	if identity == "" {
		return ""
	}
	rec, err := audit.PublicRecipientFromIdentityFile(identity)
	if err != nil {
		return ""
	}
	return rec
}
