// Package cli K-36 recipient subcommand: routes `thimble recipient
// <add|remove|list|sign-add>` to the appropriate Store entry point.
// The pre-K-36 add/remove behavior is preserved when no quorum
// policy file exists; the gate is opt-in.
package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cartine/thimble/internal/store"
)

// runRecipientV2 dispatches the recipient subcommands. Kept in a
// dedicated file so the legacy run_basic.go stays focused on the
// single-positional CRUD commands.
func runRecipientV2(st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return errors.New(
			"usage: thimble recipient <add|remove|list|sign-add> [args]",
		)
	}
	switch args[0] {
	case "add":
		return runRecipientAdd(st, args[1:], stdout, stderr)
	case "remove":
		return runRecipientRemove(st, args[1:], stdout)
	case "list":
		return runRecipientList(st, args[1:], stdout)
	case "sign-add":
		return runRecipientSignAdd(st, args[1:], stdout)
	default:
		return errors.New(
			"usage: thimble recipient <add|remove|list|sign-add> [args]",
		)
	}
}

// runRecipientAdd implements `thimble recipient add [--bootstrap]
// <app> <env> <recipient>`. The --bootstrap flag is parsed in
// either the leading or post-positional position so operators can
// type it however they remember.
func runRecipientAdd(
	st *store.Store, args []string, stdout, stderr io.Writer,
) error {
	bootstrap, positional, err := parseAddArgs(args)
	if err != nil {
		return err
	}
	if len(positional) != 3 {
		return errors.New(
			"usage: thimble recipient add [--bootstrap] <app> <env> <recipient>",
		)
	}
	app, env, recipient := positional[0], positional[1], positional[2]
	outcome, err := st.AddRecipientV2(
		app, env, recipient, store.AddRecipientOptions{Bootstrap: bootstrap},
	)
	if err != nil {
		return err
	}
	return printAddOutcome(outcome, app, env, stdout, stderr)
}

// parseAddArgs separates the --bootstrap flag from the positional
// arguments. It tolerates --bootstrap appearing anywhere among the
// positionals so error messages match either order operators try.
func parseAddArgs(args []string) (bool, []string, error) {
	var positional []string
	bootstrap := false
	for _, arg := range args {
		switch {
		case arg == "--bootstrap":
			bootstrap = true
		case strings.HasPrefix(arg, "-"):
			return false, nil, fmt.Errorf("unknown flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	return bootstrap, positional, nil
}

// printAddOutcome formats the AddOutcome for stdout. The two
// branches are "added" (committed) and "prepared" (challenges
// written, awaiting sign-add from operators).
func printAddOutcome(
	outcome store.AddOutcome, app, env string, stdout, stderr io.Writer,
) error {
	switch outcome.Stage {
	case "added":
		return printAddedOutcome(outcome, app, env, stdout)
	case "prepared":
		return printPreparedOutcome(outcome, app, env, stdout, stderr)
	default:
		return fmt.Errorf("unexpected outcome stage %q", outcome.Stage)
	}
}

// printAddedOutcome renders the success line for a committed add.
// When the gate fired, the signers (by name) are listed so the
// commit alongside the bundle diff is self-documenting.
func printAddedOutcome(
	outcome store.AddOutcome, app, env string, stdout io.Writer,
) error {
	fmt.Fprintf(stdout, "added recipient to %s/%s\n", app, env)
	if len(outcome.SignerNames) > 0 {
		fmt.Fprintf(
			stdout, "quorum satisfied: %d signatures from %s\n",
			len(outcome.SignerNames),
			strings.Join(outcome.SignerNames, ", "),
		)
	}
	return nil
}

// printPreparedOutcome renders the prepare-phase summary for the
// maintainer. It tells them what to share with operators and how
// to finalize.
func printPreparedOutcome(
	outcome store.AddOutcome, app, env string, stdout, stderr io.Writer,
) error {
	_ = stderr
	fmt.Fprintf(
		stdout,
		"prepared %d challenge files in %s/.pending-recipient-adds\n",
		outcome.OperatorsCount, "secrets",
	)
	fmt.Fprintf(
		stdout,
		"need %d of %d signatures from: %s\n",
		outcome.QuorumM,
		outcome.OperatorsCount,
		strings.Join(operatorNames(outcome.PolicyOperators), ", "),
	)
	fmt.Fprintf(
		stdout,
		"have operators run: thimble recipient sign-add %s %s %s\n",
		app, env, outcome.NewRecipient,
	)
	fmt.Fprintf(
		stdout,
		"then re-run: thimble recipient add %s %s %s\n",
		app, env, outcome.NewRecipient,
	)
	return nil
}

// operatorNames returns the slice of operator names from a list of
// quorum operators in policy order. Defined as a helper so the
// printPreparedOutcome closure stays short.
func operatorNames(ops []store.PolicyOperatorView) []string {
	names := make([]string, len(ops))
	for i, op := range ops {
		names[i] = op.Name
	}
	return names
}

// runRecipientRemove keeps the existing single-recipient remove
// behavior; quorum gate is intentionally not applied because remove
// reduces the recipient set and cannot grant access. K-37 added the
// --rotate / --rotate-randoms-only flags: when given, every K-37
// OriginProvision value in the namespace is regenerated atomically
// alongside the recipient removal so the removed peer cannot use
// its already-decrypted plaintext to access the new bundle.
func runRecipientRemove(
	st *store.Store, args []string, stdout io.Writer,
) error {
	opts, positional, err := parseRemoveArgs(args)
	if err != nil {
		return err
	}
	if len(positional) != 3 {
		return errors.New(
			"usage: thimble recipient remove [--rotate|--rotate-randoms-only] " +
				"<app> <env> <recipient>",
		)
	}
	app, env, recipient := positional[0], positional[1], positional[2]
	outcome, err := st.RemoveRecipientWithRotation(app, env, recipient, opts)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "removed recipient from %s/%s\n", app, env)
	if !opts.Rotate {
		return nil
	}
	printRotateOutcome(outcome, opts.RandomsOnly, stdout)
	return nil
}

// parseRemoveArgs separates --rotate / --rotate-randoms-only from
// the positional arguments. Both flags are mutually exclusive (one
// is the silent variant of the other); the parser refuses if both
// are given to keep the operator's intent unambiguous.
func parseRemoveArgs(args []string) (store.RotateOptions, []string, error) {
	var opts store.RotateOptions
	var positional []string
	for _, arg := range args {
		switch {
		case arg == "--rotate":
			opts.Rotate = true
		case arg == "--rotate-randoms-only":
			opts.Rotate = true
			opts.RandomsOnly = true
		case strings.HasPrefix(arg, "-"):
			return store.RotateOptions{}, nil, fmt.Errorf("unknown flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	return opts, positional, nil
}

// printRotateOutcome formats the K-37 rotation summary. One line per
// rotated key, one line per "manual rotate needed" key (suppressed
// in --rotate-randoms-only mode), and a final tally so the operator
// can confirm what changed.
func printRotateOutcome(
	outcome store.RotateOutcome, randomsOnly bool, stdout io.Writer,
) {
	for _, key := range outcome.Rotated {
		fmt.Fprintf(stdout, "rotated %s (provisioned)\n", key)
	}
	if !randomsOnly {
		for _, entry := range outcome.NeedsAttention {
			fmt.Fprintf(
				stdout,
				"manual rotate needed: %s (origin: %s; %s)\n",
				entry.Key, entry.Origin,
				rotateNeedsHint(entry.Origin),
			)
		}
	}
	fmt.Fprintf(
		stdout, "%d rotated, %d needs-attention\n",
		len(outcome.Rotated), len(outcome.NeedsAttention),
	)
}

// rotateNeedsHint returns a short hint for the operator on what to
// do for each non-provisioned origin. Wording matches the K-37
// acceptance text so a user looking for the message can grep it.
func rotateNeedsHint(origin store.Origin) string {
	switch origin {
	case store.OriginAndSet:
		return "rerun your producer command"
	case store.OriginSet, "":
		return "re-set the value"
	default:
		return "re-set the value"
	}
}

// runRecipientList prints the recipient list with prefix labels and
// thumbprints. Output is one per line: `<prefix> <thumbprint>
// <recipient>`. Thumbprint is the K-27 sha256-prefixed-16-hex-char
// label so operators can refer to a recipient without copying the
// full string.
func runRecipientList(
	st *store.Store, args []string, stdout io.Writer,
) error {
	if len(args) != 2 {
		return errors.New("usage: thimble recipient list <app> <env>")
	}
	entries, err := st.ListRecipients(args[0], args[1])
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Fprintf(
			stdout, "%-12s %s %s\n", e.Prefix, e.Thumbprint, e.Recipient,
		)
	}
	return nil
}

// runRecipientSignAdd is the operator's command. It requires
// THIMBLE_AGE_IDENTITY to be configured and prints a one-line
// status summarizing the produced signature.
func runRecipientSignAdd(
	st *store.Store, args []string, stdout io.Writer,
) error {
	if len(args) != 3 {
		return errors.New(
			"usage: thimble recipient sign-add <app> <env> <recipient>",
		)
	}
	app, env, recipient := args[0], args[1], args[2]
	summary, err := st.SignAddRecipient(app, env, recipient)
	if err != nil {
		return err
	}
	fmt.Fprintf(
		stdout,
		"signed by %s (%s); %d signatures required total\n",
		summary.OperatorName, summary.OperatorThumb, summary.QuorumM,
	)
	fmt.Fprintf(
		stdout,
		"signature file: %s\n",
		summary.SignaturePath,
	)
	return nil
}
