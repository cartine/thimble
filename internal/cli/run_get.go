package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/cartine/thimble/internal/store"
)

// runGet prints one decrypted value (with KEY) or, with KEY omitted,
// the sorted key names of the namespace — same output as `list`;
// values never reach stdout in that mode.
func runGet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 && len(rest) != 3 {
		return errors.New("usage: thimble get <app> <env> [KEY]")
	}
	app, env := rest[0], rest[1]
	if len(rest) == 2 {
		return printKeyNames(st, app, env, stdout)
	}
	key := rest[2]
	values, _, err := st.ReadEnv(app, env)
	if err != nil {
		return err
	}
	value, ok := values[key]
	if !ok {
		return fmt.Errorf("%s is not set in %s/%s", key, app, env)
	}
	_, err = fmt.Fprintln(stdout, value)
	return err
}
