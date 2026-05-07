package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/cartine/thimble/internal/audit"
	"github.com/cartine/thimble/internal/store"
)

// runAudit implements `thimble audit <app> <env>` per K-27. It reads
// the audit log under the configured store root, filters to the
// supplied namespace, and prints a tabular view of the most recent
// --limit entries (default 50).
func runAudit(st *store.Store, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	limit := fs.Int("limit", 50, "max number of recent entries to print")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return errors.New("usage: thimble audit [--limit N] <app> <env>")
	}
	if *limit < 1 {
		return errors.New("--limit must be >= 1")
	}
	events, err := audit.Read(st.Root())
	if err != nil {
		return err
	}
	filtered := audit.Filter(events, rest[0], rest[1])
	printAuditEntries(stdout, filtered, *limit)
	return nil
}

// printAuditEntries renders up to limit most recent events as a
// tab-aligned table. Older entries are dropped, not the new ones,
// because operators almost always want the tail.
func printAuditEntries(w io.Writer, events []audit.Event, limit int) {
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "time\toperator\top\tsubject")
	for _, e := range events {
		ts := e.Timestamp.UTC().Format(time.RFC3339)
		subject := e.Subject
		if subject == "" {
			subject = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", ts, e.Operator, e.Op, subject)
	}
	tw.Flush()
}
