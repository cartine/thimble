package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cartine/thimble/internal/store"
)

func runInit(st *store.Store, args []string, stdout, stderr io.Writer) error {
	_ = stderr
	recipients, positional, err := parseInitArgs(args)
	if err != nil {
		return err
	}
	if len(positional) != 2 {
		return errors.New("usage: thimble init <app> <env> --recipient age1...")
	}
	if len(recipients) == 0 {
		return errors.New("init requires at least one --recipient")
	}
	app, env := positional[0], positional[1]
	if err := st.Init(app, env, recipients); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "initialized %s/%s\n", app, env)
	return nil
}

func parseInitArgs(args []string) (recipients, positional []string, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--recipient":
			if i+1 >= len(args) {
				return nil, nil, errors.New("--recipient requires a value")
			}
			recipients = append(recipients, args[i+1])
			i++
		case strings.HasPrefix(arg, "--recipient="):
			recipients = append(recipients, strings.TrimPrefix(arg, "--recipient="))
		case strings.HasPrefix(arg, "-"):
			return nil, nil, fmt.Errorf("unknown init flag %q", arg)
		default:
			positional = append(positional, arg)
		}
	}
	return recipients, positional, nil
}

// runWrite covers both `create` (requireExisting=false) and `update`
// (requireExisting=true). The third positional may not be a value;
// secret values arrive on stdin or via the masked prompt. The hidden
// --origin flag (K-37) labels the source so a later
// `recipient remove --rotate` can skip operator-supplied values.
func runWrite(
	st *store.Store, args []string, stdout, stderr io.Writer, requireExisting bool,
) error {
	origin, positional, err := parseSecretArgs(args, requireExisting, false)
	if err != nil {
		return err
	}
	app, env, key := positional[0], positional[1], positional[2]
	value, err := secretInput(key, stderr)
	if err != nil {
		return err
	}
	if requireExisting {
		err = st.UpdateSecretWithOrigin(app, env, key, value, origin)
	} else {
		err = st.CreateSecretWithOrigin(app, env, key, value, origin)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s\n", key, app, env)
	return nil
}

// parseSecretArgs splits --origin from the positional arguments and
// validates that exactly three positionals (app, env, KEY) remain. The
// "secret as positional" guard fires when more than three positionals
// are given. requireExisting and forSet only affect the usage message.
func parseSecretArgs(
	args []string, requireExisting, forSet bool,
) (store.Origin, []string, error) {
	originRaw, positional, err := splitOriginFlag(args)
	if err != nil {
		return "", nil, err
	}
	if len(positional) > 3 {
		return "", nil, errors.New(
			"do not pass secret values as arguments; pipe stdin or use the masked prompt",
		)
	}
	if len(positional) != 3 {
		return "", nil, errors.New(secretUsageString(requireExisting, forSet))
	}
	origin, err := store.ParseOrigin(originRaw)
	if err != nil {
		return "", nil, err
	}
	return origin, positional, nil
}

// secretUsageString returns the right "usage" line for the calling
// subcommand. Centralizing it keeps run_basic.go tidy when --origin is
// added to all three of create/update/set.
func secretUsageString(requireExisting, forSet bool) string {
	switch {
	case forSet:
		return "usage: thimble set [--origin=set|provision|and-set] <app> <env> <KEY>"
	case requireExisting:
		return "usage: thimble update [--origin=set|provision|and-set] <app> <env> <KEY>"
	default:
		return "usage: thimble create [--origin=set|provision|and-set] <app> <env> <KEY>"
	}
}

// splitOriginFlag extracts --origin/--origin=value/-origin from args
// and returns the rest as positionals. Unknown flags fall through to
// the positional list so the existing "do not pass values" guard can
// still react to obvious misuse.
func splitOriginFlag(args []string) (string, []string, error) {
	origin := ""
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--origin":
			if i+1 >= len(args) {
				return "", nil, errors.New("--origin requires a value")
			}
			origin = args[i+1]
			i++
		case strings.HasPrefix(arg, "--origin="):
			origin = strings.TrimPrefix(arg, "--origin=")
		default:
			positional = append(positional, arg)
		}
	}
	return origin, positional, nil
}

func runSet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	origin, positional, err := parseSecretArgs(args, false, true)
	if err != nil {
		return err
	}
	app, env, key := positional[0], positional[1], positional[2]
	value, err := secretInput(key, stderr)
	if err != nil {
		return err
	}
	if err := st.SetSecretWithOrigin(app, env, key, value, origin); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s\n", key, app, env)
	return nil
}

func runDelete(st *store.Store, args []string, stdout io.Writer) error {
	if len(args) != 3 {
		return errors.New("usage: thimble delete <app> <env> <KEY>")
	}
	app, env, key := args[0], args[1], args[2]
	if err := st.DeleteSecret(app, env, key); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deleted %s from %s/%s\n", key, app, env)
	return nil
}

func runList(st *store.Store, args []string, stdout io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: thimble list <app> <env>")
	}
	keys, err := st.ListSecrets(args[0], args[1])
	if err != nil {
		return err
	}
	for _, key := range keys {
		fmt.Fprintln(stdout, key)
	}
	return nil
}
