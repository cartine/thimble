package cli

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/cartine/thimble/internal/store"
)

func runProvision(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("provision", flag.ContinueOnError)
	fs.SetOutput(stderr)
	byteCount := fs.Int("bytes", 32, "random byte count before encoding")
	show := fs.Bool("show", false, "allow writing the generated secret to a terminal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: thimble provision [--bytes 32]")
	}
	if *byteCount < 16 {
		return errors.New("provision requires at least 16 bytes")
	}
	if writerIsTerminal(stdout) && !*show {
		return errors.New(
			"refusing to print a new secret to the terminal; pipe it or pass --show",
		)
	}
	b := make([]byte, *byteCount)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	_, err := fmt.Fprintln(stdout, base64.RawURLEncoding.EncodeToString(b))
	return err
}

func runAndSet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) < 5 {
		return errors.New("usage: thimble and-set <app> <env> <KEY> -- <command> [args...]")
	}
	app, env, key := args[0], args[1], args[2]
	cmdArgs, err := commandAfterDash(args[3:])
	if err != nil {
		return err
	}
	value, err := runSecretProducer(cmdArgs, stderr)
	if err != nil {
		return err
	}
	if err := st.SetSecret(app, env, key, value); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "saved %s in %s/%s from command output\n", key, app, env)
	return nil
}

func runAndGet(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("and-get", flag.ContinueOnError)
	fs.SetOutput(stderr)
	envVar := fs.String("env", "", "also expose the secret as this environment variable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) < 5 {
		return errors.New(
			"usage: thimble and-get [--env NAME] <app> <env> <KEY> -- <command> [args...]",
		)
	}
	app, env, key := rest[0], rest[1], rest[2]
	cmdArgs, err := commandAfterDash(rest[3:])
	if err != nil {
		return err
	}
	values, _, err := st.ReadEnv(app, env)
	if err != nil {
		return err
	}
	value, ok := values[key]
	if !ok {
		return fmt.Errorf("%s does not exist", key)
	}
	return runSecretConsumer(cmdArgs, value, *envVar, stdout, stderr)
}

func runRender(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "dotenv", "output format; only dotenv is supported")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 2 {
		return errors.New("usage: thimble render <app> <env> --format dotenv")
	}
	if *format != "dotenv" {
		return errors.New("only dotenv render format is supported")
	}
	plain, err := st.Render(fs.Arg(0), fs.Arg(1))
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, plain)
	return err
}
