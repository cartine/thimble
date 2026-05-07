package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/doctor"
	"github.com/cartine/thimble/internal/store"
)

// runDoctor implements `thimble doctor` per K-29. It runs every
// diagnostic in doctor.Run, prints a tabular report to stdout, and
// returns a non-nil error if any check failed so the CLI exits
// non-zero. --json swaps the renderer for newline-terminated JSON.
func runDoctor(
	ctx context.Context,
	st *store.Store,
	tool *age.Tool,
	args []string,
	stdout, stderr io.Writer,
	cfg cliConfig,
) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit machine-readable JSON")
	addr := fs.String("addr", "", "web port to probe (default 127.0.0.1:8787)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: thimble doctor [--json] [--addr 127.0.0.1:8787]")
	}
	report := doctor.Run(ctx, st, tool, doctor.Options{
		WebAddr:      *addr,
		IdentityPath: cfg.identity,
		AgeSHA256Pin: cfg.ageSHA256,
	})
	if *jsonOut {
		if err := writeJSON(stdout, report); err != nil {
			return err
		}
	} else {
		writeDoctorTable(stdout, report)
	}
	if report.HasFailures() {
		return errors.New("doctor found one or more failing checks")
	}
	return nil
}

func writeJSON(w io.Writer, report doctor.Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

func writeDoctorTable(w io.Writer, report doctor.Report) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "check\tstatus\tdetail")
	for _, c := range report.Checks {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, c.Status, c.Detail)
	}
	tw.Flush()
}
