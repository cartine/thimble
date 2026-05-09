package quorum

import (
	"context"
	"errors"
	"fmt"
	"os"
)

// SignAddInputs collects the parameters SignAdd needs. The operator's
// public recipient (parsed from their age identity file by the CLI
// layer) is used to locate their challenge file by thumbprint.
type SignAddInputs struct {
	StoreRoot         string
	App               string
	Env               string
	NewRecipient      string
	OperatorRecipient string
	Age               AgeTool
}

// SignResult is what SignAdd returns to the CLI on success. The CLI
// formats this into the friendly "1 of M signatures collected"
// message.
type SignResult struct {
	OperatorThumb     string
	SignaturePath     string
	VerifierRecipient string
	QuorumM           int
}

// SignAdd is run by an operator with their identity configured. It
// (a) loads meta.json to find their challenge, (b) decrypts the
// challenge with their identity (proving key possession),
// (c) re-encrypts the same canonical message to the verifier's
// recipient and writes the signature file, (d) returns metadata for
// the CLI's status line. Returning an error leaves the pending
// directory unchanged so the operator can retry.
func SignAdd(ctx context.Context, in SignAddInputs) (SignResult, error) {
	if err := validateSignInputs(in); err != nil {
		return SignResult{}, err
	}
	meta, present, err := LoadMeta(in.StoreRoot)
	if err != nil {
		return SignResult{}, err
	}
	if !present {
		return SignResult{}, fmt.Errorf(
			"no pending recipient add at %s; "+
				"have a maintainer run `thimble recipient add` first to prepare",
			MetaPath(in.StoreRoot),
		)
	}
	if err := checkSignMatchesMeta(meta, in); err != nil {
		return SignResult{}, err
	}
	thumb := RecipientThumbprint(in.OperatorRecipient)
	if _, ok := findOperatorByThumb(meta.PolicyOperators, thumb); !ok {
		return SignResult{}, fmt.Errorf(
			"operator with recipient thumbprint %s is not in the policy "+
				"operators list (run on the wrong machine?)",
			thumb,
		)
	}
	challengePath := ChallengePath(in.StoreRoot, thumb)
	if _, err := os.Stat(challengePath); err != nil {
		return SignResult{}, fmt.Errorf(
			"challenge file missing for operator %s; rerun prepare", thumb,
		)
	}
	plain, err := in.Age.Decrypt(ctx, challengePath)
	if err != nil {
		return SignResult{}, fmt.Errorf(
			"decrypt challenge: %w "+
				"(is THIMBLE_AGE_IDENTITY pointing at the operator's identity?)",
			err,
		)
	}
	if err := CheckCanonical(
		plain, meta.App, meta.Env, meta.NewRecipient, meta.BundleSHA, meta.Nonce,
	); err != nil {
		return SignResult{}, err
	}
	sig, err := in.Age.Encrypt(
		ctx, []string{meta.VerifierRecipient}, plain,
	)
	if err != nil {
		return SignResult{}, fmt.Errorf("encrypt signature: %w", err)
	}
	sigPath := SignaturePath(in.StoreRoot, thumb)
	if err := writePending(sigPath, sig); err != nil {
		return SignResult{}, err
	}
	return SignResult{
		OperatorThumb:     thumb,
		SignaturePath:     sigPath,
		VerifierRecipient: meta.VerifierRecipient,
		QuorumM:           meta.QuorumM,
	}, nil
}

func validateSignInputs(in SignAddInputs) error {
	if in.StoreRoot == "" {
		return errors.New("store root is empty")
	}
	if in.OperatorRecipient == "" {
		return errors.New(
			"operator recipient is empty; configure THIMBLE_AGE_IDENTITY",
		)
	}
	if in.NewRecipient == "" {
		return errors.New("new recipient is empty")
	}
	if in.Age == nil {
		return errors.New("age tool is nil")
	}
	return nil
}

// checkSignMatchesMeta refuses if the operator is signing a different
// recipient or namespace than the one prepare-add committed to. This
// is a UX guardrail; the canonical-message comparison after decrypt
// is the cryptographic check.
func checkSignMatchesMeta(meta Meta, in SignAddInputs) error {
	if meta.NewRecipient != in.NewRecipient {
		return fmt.Errorf(
			"pending add is for recipient %s but you supplied %s",
			meta.NewRecipient, in.NewRecipient,
		)
	}
	if meta.App != in.App || meta.Env != in.Env {
		return fmt.Errorf(
			"pending add is for %s/%s but you supplied %s/%s",
			meta.App, meta.Env, in.App, in.Env,
		)
	}
	return nil
}

// findOperatorByThumb returns the operator entry whose recipient
// hashes to thumb, or (Operator{}, false) if none. Used both at
// sign time and verify time so the lookup logic stays in one place.
func findOperatorByThumb(ops []Operator, thumb string) (Operator, bool) {
	for _, op := range ops {
		if RecipientThumbprint(op.Recipient) == thumb {
			return op, true
		}
	}
	return Operator{}, false
}
