package quorum

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VerifyInputs collects the parameters Verify needs.
type VerifyInputs struct {
	StoreRoot    string
	App          string
	Env          string
	NewRecipient string
	BundleSHA    string
	Policy       Policy
	Age          AgeTool
}

// VerifyResult is what Verify returns to the store layer on success.
// SignerThumbs is the list of distinct policy operator thumbprints
// whose signatures were valid; the audit hook records these.
type VerifyResult struct {
	SignerThumbs []string
	SignerNames  []string
}

// Verify enforces the M-of-N quorum at recipient-add time. It
// re-reads meta.json, ensures the meta still matches the pending
// add, then decrypts every .sig file with the maintainer's identity
// (via the supplied AgeTool's configured identity) and checks each
// plaintext equals the canonical message. Distinct, listed
// operators are counted; ≥M is success.
//
// On insufficient signatures, an error explaining how many were
// present and which operators are still missing is returned. The
// pending directory is NOT cleared on failure so a maintainer can
// collect the remaining signatures and retry.
func Verify(ctx context.Context, in VerifyInputs) (VerifyResult, error) {
	meta, ok, err := LoadMeta(in.StoreRoot)
	if err != nil {
		return VerifyResult{}, err
	}
	if !ok {
		return VerifyResult{}, fmt.Errorf(
			"no pending recipient add at %s; "+
				"prepare one with `thimble recipient add` to start the quorum flow",
			MetaPath(in.StoreRoot),
		)
	}
	if err := checkMetaMatches(meta, in); err != nil {
		return VerifyResult{}, err
	}
	signers, err := collectValidSignatures(ctx, in.StoreRoot, meta, in.Age)
	if err != nil {
		return VerifyResult{}, err
	}
	if len(signers) < meta.QuorumM {
		return VerifyResult{}, fmt.Errorf(
			"%d/%d signatures present (need from %s)",
			len(signers), meta.QuorumM,
			describeMissing(meta.PolicyOperators, signers),
		)
	}
	thumbs := make([]string, len(signers))
	names := make([]string, len(signers))
	for i, op := range signers {
		thumbs[i] = RecipientThumbprint(op.Recipient)
		names[i] = op.Name
	}
	return VerifyResult{SignerThumbs: thumbs, SignerNames: names}, nil
}

// checkMetaMatches refuses if the recipient or namespace the
// maintainer supplied at add time differs from what was prepared,
// and if the on-disk bundle has been re-encrypted since prepare
// (replay defence).
func checkMetaMatches(meta Meta, in VerifyInputs) error {
	if meta.App != in.App || meta.Env != in.Env {
		return fmt.Errorf(
			"pending add is for %s/%s; cannot finalize against %s/%s",
			meta.App, meta.Env, in.App, in.Env,
		)
	}
	if meta.NewRecipient != in.NewRecipient {
		return fmt.Errorf(
			"pending add is for recipient %s; cannot finalize against %s",
			meta.NewRecipient, in.NewRecipient,
		)
	}
	if meta.BundleSHA != in.BundleSHA {
		return fmt.Errorf(
			"bundle SHA changed since prepare (was %s; now %s); "+
				"signatures are stale, re-prepare and re-sign",
			meta.BundleSHA, in.BundleSHA,
		)
	}
	if meta.QuorumM != in.Policy.M {
		return fmt.Errorf(
			"policy quorum_m changed since prepare (was %d; now %d); re-prepare",
			meta.QuorumM, in.Policy.M,
		)
	}
	return nil
}

// collectValidSignatures walks the pending directory's .sig files,
// decrypts each with the maintainer's identity, checks the canonical
// message, dedupes by thumbprint, and returns the matching policy
// operators. A signature whose thumbprint is not in the policy or
// whose plaintext does not match is silently ignored — the count
// failure path renders the user-visible error.
func collectValidSignatures(
	ctx context.Context, storeRoot string, meta Meta, tool AgeTool,
) ([]Operator, error) {
	entries, err := os.ReadDir(PendingDir(storeRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read pending dir: %w", err)
	}
	seen := map[string]bool{}
	var out []Operator
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, sigSuffix) {
			continue
		}
		thumb := strings.TrimSuffix(name, sigSuffix)
		if seen[thumb] {
			continue
		}
		op, ok := findOperatorByThumb(meta.PolicyOperators, thumb)
		if !ok {
			continue
		}
		path := filepath.Join(PendingDir(storeRoot), name)
		plain, err := tool.Decrypt(ctx, path)
		if err != nil {
			continue
		}
		if err := CheckCanonical(
			plain, meta.App, meta.Env, meta.NewRecipient,
			meta.BundleSHA, meta.Nonce,
		); err != nil {
			continue
		}
		seen[thumb] = true
		out = append(out, op)
	}
	return out, nil
}

// describeMissing turns the policy minus the present signers into a
// human-readable list joined by " or " for the final error message.
func describeMissing(all []Operator, present []Operator) string {
	got := map[string]bool{}
	for _, op := range present {
		got[op.Name] = true
	}
	var names []string
	for _, op := range all {
		if !got[op.Name] {
			names = append(names, op.Name)
		}
	}
	if len(names) == 0 {
		return "(no operators remaining)"
	}
	return strings.Join(names, " or ")
}
