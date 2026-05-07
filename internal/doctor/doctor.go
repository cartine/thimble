// Package doctor implements `thimble doctor` (K-29): a one-shot
// health check that surfaces every common Thimble pitfall before an
// operator hits it in production. Each diagnostic is a typed
// CheckResult; the report is the ordered slice of those results, and
// the exit code is non-zero if any check has Status == StatusFail.
package doctor

import (
	"context"

	"github.com/cartine/thimble/internal/age"
	"github.com/cartine/thimble/internal/store"
)

// Status is the verdict from a single diagnostic.
type Status string

const (
	// StatusOK means the diagnostic passed; no action needed.
	StatusOK Status = "ok"
	// StatusWarn means the diagnostic found a non-blocking issue; the
	// operator should look but Thimble can still run.
	StatusWarn Status = "warn"
	// StatusFail means the diagnostic found a blocking issue. Doctor
	// exits non-zero and the operator must fix it before running.
	StatusFail Status = "fail"
)

// IsOK returns true if s == StatusOK. Convenience for terse test
// assertions and template rendering.
func (s Status) IsOK() bool { return s == StatusOK }

// CheckResult is the outcome of one diagnostic in the doctor report.
type CheckResult struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
}

// Report is the ordered list of diagnostics produced by Run.
type Report struct {
	Checks []CheckResult `json:"checks"`
}

// HasFailures returns true if any check in the report is StatusFail.
func (r Report) HasFailures() bool {
	for _, c := range r.Checks {
		if c.Status == StatusFail {
			return true
		}
	}
	return false
}

// Options tunes optional aspects of doctor (e.g. the web UI port to
// probe). Zero-valued Options are sensible defaults.
type Options struct {
	// WebAddr is the bind address the web check probes. Empty
	// defaults to 127.0.0.1:8787.
	WebAddr string
	// IdentityPath is the operator's identity file path, if
	// configured. Doctor checks its mode and existence.
	IdentityPath string
	// AgeSHA256Pin is the optional SHA-256 pin from
	// $THIMBLE_AGE_SHA256. Doctor reports whether the resolved age
	// binary matches.
	AgeSHA256Pin string
}

// Run executes every diagnostic, in order, and returns the report.
// The order is fixed and matches the spec: age, identity, store,
// manifest, bundles, web port, recipients.
func Run(ctx context.Context, st *store.Store, tool *age.Tool, opts Options) Report {
	_ = ctx // reserved for future cancellation
	checks := []CheckResult{
		checkAge(tool, opts),
		checkIdentity(opts),
		checkStoreDir(st),
		checkManifest(st),
		checkBundles(st),
		checkWebPort(opts),
	}
	checks = append(checks, checkRecipientsList(st)...)
	return Report{Checks: checks}
}
