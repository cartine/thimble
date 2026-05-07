package quorum

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// PrepareInputs bundles the parameters PrepareAdd needs. Splitting
// the input into a struct keeps the public function signature
// short and lets the store layer construct it without naming each
// field at the call site.
type PrepareInputs struct {
	StoreRoot         string
	App               string
	Env               string
	NewRecipient      string
	BundleSHA         string
	Policy            Policy
	VerifierRecipient string
	Age               AgeTool
}

// PrepareAdd writes meta.json and one challenge file per operator
// listed in Policy.Operators. Each challenge is age-ciphertext of
// the canonical message, addressed to that operator alone. Operators
// run SignAdd against their identity to consume their challenge and
// produce a matching signature file.
//
// PrepareAdd is idempotent in the sense that re-running it overwrites
// meta.json and the challenge files (replacing any stale set), but
// callers should ensure no prior add is in flight; the store layer
// holds the namespace lock to enforce this.
func PrepareAdd(ctx context.Context, in PrepareInputs) error {
	if err := validatePrepareInputs(in); err != nil {
		return err
	}
	nonce, err := NewNonce()
	if err != nil {
		return err
	}
	canonical := CanonicalMessage(
		in.App, in.Env, in.NewRecipient, in.BundleSHA, nonce,
	)
	if err := os.MkdirAll(PendingDir(in.StoreRoot), 0o700); err != nil {
		return fmt.Errorf("create pending dir: %w", err)
	}
	for _, op := range in.Policy.Operators {
		thumb := RecipientThumbprint(op.Recipient)
		ciphertext, err := in.Age.Encrypt(
			ctx, []string{op.Recipient}, canonical,
		)
		if err != nil {
			return fmt.Errorf(
				"encrypt challenge for %q: %w", op.Name, err,
			)
		}
		path := ChallengePath(in.StoreRoot, thumb)
		if err := writePending(path, ciphertext); err != nil {
			return err
		}
	}
	meta := Meta{
		Version:           1,
		App:               in.App,
		Env:               in.Env,
		NewRecipient:      in.NewRecipient,
		BundleSHA:         in.BundleSHA,
		Nonce:             nonce,
		VerifierRecipient: in.VerifierRecipient,
		PolicyOperators:   in.Policy.Operators,
		QuorumM:           in.Policy.M,
	}
	return SaveMeta(in.StoreRoot, meta)
}

// validatePrepareInputs sanity-checks the inputs so callers see a
// clear error before any IO happens. It does NOT re-validate the
// policy itself — LoadPolicy already did that.
func validatePrepareInputs(in PrepareInputs) error {
	if in.StoreRoot == "" {
		return errors.New("store root is empty")
	}
	if in.App == "" || in.Env == "" {
		return errors.New("app and env are required")
	}
	if in.NewRecipient == "" {
		return errors.New("new recipient is empty")
	}
	if in.BundleSHA == "" {
		return errors.New("bundle SHA is empty")
	}
	if in.VerifierRecipient == "" {
		return errors.New("verifier recipient is empty")
	}
	if in.Age == nil {
		return errors.New("age tool is nil")
	}
	if len(in.Policy.Operators) == 0 {
		return errors.New("policy operators list is empty")
	}
	for _, op := range in.Policy.Operators {
		if op.Recipient == in.NewRecipient {
			return fmt.Errorf(
				"operator %q has the same recipient as the new add; "+
					"refusing to prepare",
				op.Name,
			)
		}
	}
	return nil
}

// writePending writes contents to path with mode 0o600. We do not
// need atomicity here because each pending file is consumed by a
// single later step; a crashed PrepareAdd is recovered by re-running
// PrepareAdd, which overwrites.
func writePending(path string, contents []byte) error {
	// #nosec G304 -- path is constructed from the configured store
	// root and a fixed filename pattern; not user input.
	f, err := os.OpenFile(
		path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600,
	)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}
