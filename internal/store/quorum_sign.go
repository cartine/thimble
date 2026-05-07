// K-36 sign-add and recipient-list helpers exposed to the CLI. The
// thin wrappers keep the heavy lifting in internal/quorum so the
// store layer just owns identity resolution and lock acquisition.
package store

import (
	"errors"
	"fmt"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/quorum"
)

// SignAddSummary is what SignAddRecipient returns to the CLI for
// display. M is the policy's required signature count; OperatorThumb
// is the operator's own opaque ID for the printed message.
type SignAddSummary struct {
	OperatorThumb string
	OperatorName  string
	SignaturePath string
	QuorumM       int
}

// SignAddRecipient is the operator-side counterpart to
// AddRecipientV2. It loads the policy and the pending meta.json,
// finds the operator's challenge by their identity's thumbprint,
// decrypts it (proving key possession), and re-encrypts the
// canonical message to the maintainer's verifier recipient. The
// produced signature file is later consumed by Verify.
func (s *Store) SignAddRecipient(
	app, env, newRecipient string,
) (SignAddSummary, error) {
	cleaned, err := CleanRecipient(newRecipient)
	if err != nil {
		return SignAddSummary{}, err
	}
	policy, present, err := quorum.LoadPolicy(quorum.PolicyPath(s.root))
	if err != nil {
		return SignAddSummary{}, err
	}
	if !present {
		return SignAddSummary{}, errors.New(
			"sign-add requires recipients.signed.toml to be present " +
				"(no quorum policy is configured)",
		)
	}
	op, err := s.requireOperatorIdentity(policy)
	if err != nil {
		return SignAddSummary{}, err
	}
	result, err := quorum.SignAdd(s.context(), quorum.SignAddInputs{
		StoreRoot:         s.root,
		App:               app,
		Env:               env,
		NewRecipient:      cleaned,
		OperatorRecipient: op.Recipient,
		Age:               s.age,
	})
	if err != nil {
		return SignAddSummary{}, err
	}
	return SignAddSummary{
		OperatorThumb: result.OperatorThumb,
		OperatorName:  op.Name,
		SignaturePath: result.SignaturePath,
		QuorumM:       result.QuorumM,
	}, nil
}

// requireOperatorIdentity checks that the maintainer's identity
// resolves to a recipient that appears in the policy. Returns a
// targeted error otherwise so a misconfigured shell does not
// silently produce an unsignable artifact.
func (s *Store) requireOperatorIdentity(
	policy quorum.Policy,
) (quorum.Operator, error) {
	if s.age == nil {
		return quorum.Operator{}, errors.New("age tool is not configured")
	}
	identity := s.age.Identity()
	if identity == "" {
		return quorum.Operator{}, errors.New(
			"sign-add requires THIMBLE_AGE_IDENTITY (operator's identity)",
		)
	}
	pub, err := audit.PublicRecipientFromIdentityFile(identity)
	if err != nil {
		return quorum.Operator{}, fmt.Errorf("read identity: %w", err)
	}
	op, ok := policy.FindByRecipient(pub)
	if !ok {
		return quorum.Operator{}, fmt.Errorf(
			"operator recipient %s is not in recipients.signed.toml",
			audit.Thumbprint(pub),
		)
	}
	return op, nil
}

// RecipientListEntry is one row in ListRecipients' output: the raw
// recipient string, its prefix label (age1 / ssh-ed25519 / ssh-rsa),
// and its opaque thumbprint. Used by the K-36 `recipient list`
// subcommand.
type RecipientListEntry struct {
	Prefix     string
	Recipient  string
	Thumbprint string
}

// ListRecipients returns the recipient list for (app, env) with
// thumbprints attached. Reuses Find under a shared flock so it is
// concurrent-safe with respect to writers.
func (s *Store) ListRecipients(app, env string) ([]RecipientListEntry, error) {
	meta, err := s.Find(app, env)
	if err != nil {
		return nil, err
	}
	out := make([]RecipientListEntry, 0, len(meta.Recipients))
	for _, r := range meta.Recipients {
		out = append(out, RecipientListEntry{
			Prefix:     recipientPrefix(r),
			Recipient:  r,
			Thumbprint: audit.Thumbprint(r),
		})
	}
	return out, nil
}
