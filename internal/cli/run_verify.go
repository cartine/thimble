package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/cartine/thimble/internal/store"
)

// runVerify implements `thimble verify <app> <env>`. Per K-22 it
// recomputes the bundle's SHA-256, compares against the manifest's
// stored value, and prints a tabular report. It exits with a non-zero
// error when the SHAs do not match (or the manifest is older and has
// no stored SHA at all).
func runVerify(st *store.Store, args []string, stdout, stderr io.Writer) error {
	_ = stderr
	if len(args) != 2 {
		return errors.New("usage: thimble verify <app> <env>")
	}
	report, err := st.Verify(args[0], args[1])
	if err != nil {
		return err
	}
	printVerifyReport(stdout, report)
	if !report.Match {
		if report.ManifestSHA == "" {
			return fmt.Errorf(
				"%s/%s: manifest has no BundleSHA256; "+
					"run a mutation (e.g. thimble set) to upgrade",
				report.App, report.Env,
			)
		}
		return fmt.Errorf(
			"%s/%s: SHA-256 MISMATCH (manifest=%s file=%s)",
			report.App, report.Env, report.ManifestSHA, report.ActualSHA,
		)
	}
	return nil
}

// printVerifyReport renders a VerifyReport in a stable tabular form.
// Bundle path, size, and mode go first; then SHA verdict; then
// recipient list with prefix labels.
func printVerifyReport(w io.Writer, r store.VerifyReport) {
	verdict := "match"
	if !r.Match {
		verdict = "MISMATCH"
	}
	fmt.Fprintf(w, "namespace:    %s/%s\n", r.App, r.Env)
	fmt.Fprintf(w, "bundle:       %s\n", r.BundlePath)
	fmt.Fprintf(w, "size:         %d bytes\n", r.BundleSize)
	fmt.Fprintf(w, "mode:         %#o\n", r.BundleMode)
	fmt.Fprintf(w, "version:      %d\n", r.StoredVersion)
	manifestShown := r.ManifestSHA
	if manifestShown == "" {
		manifestShown = "(empty)"
	}
	fmt.Fprintf(w, "manifest sha: %s\n", manifestShown)
	fmt.Fprintf(w, "actual sha:   %s\n", r.ActualSHA)
	fmt.Fprintf(w, "verdict:      %s\n", verdict)
	fmt.Fprintf(w, "recipients (%d):\n", len(r.Recipients))
	for _, rec := range r.Recipients {
		fmt.Fprintf(w, "  %-12s %s\n", rec.Prefix, rec.Value)
	}
}
