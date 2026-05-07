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
// secret values arrive on stdin or via the masked prompt.
func runWrite(
	st *store.Store, args []string, stdout, stderr io.Writer, requireExisting bool,
) error {
	if err := checkWriteArgs(args, requireExisting); err != nil {
		return err
	}
	app, env, key := args[0], args[1], args[2]
	value, err := secretInput(key, stderr)
	if err != nil {
		return err
	}
	if requireExisting {
		err = st.UpdateSecret(app, env, key, value)
	} else {
		err = st.CreateSecret(app, env, key, value)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s\n", key, app, env)
	return nil
}

func checkWriteArgs(args []string, requireExisting bool) error {
	if len(args) > 3 {
		return errors.New(
			"do not pass secret values as arguments; pipe stdin or use the masked prompt",
		)
	}
	if len(args) != 3 {
		if requireExisting {
			return errors.New("usage: thimble update <app> <env> <KEY>")
		}
		return errors.New("usage: thimble create <app> <env> <KEY>")
	}
	return nil
}

func runSet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) > 3 {
		return errors.New(
			"do not pass secret values as arguments; pipe stdin or use the masked prompt",
		)
	}
	if len(args) != 3 {
		return errors.New("usage: thimble set <app> <env> <KEY>")
	}
	app, env, key := args[0], args[1], args[2]
	value, err := secretInput(key, stderr)
	if err != nil {
		return err
	}
	if err := st.SetSecret(app, env, key, value); err != nil {
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
