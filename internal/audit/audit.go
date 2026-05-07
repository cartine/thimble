// Package audit owns the append-only ledger of mutating operations
// recorded by Thimble (K-27). Entries are JSON Lines written to
// <store>/.thimble-audit.log; the file is mode 0640 on first creation
// and opened with O_APPEND so concurrent writers do not tear each
// other's records. The ledger is intentionally minimal: timestamps,
// an opaque operator thumbprint, and the operation's namespace and
// subject — never values, never identity file paths, never the full
// recipient string by itself.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Operation names recorded in the audit log. Keeping them as named
// constants both documents the vocabulary and prevents typo drift
// across packages that call Logger.Append.
const (
	OpInit            = "init"
	OpRecipientAdd    = "recipient_add"
	OpRecipientRemove = "recipient_remove"
	OpCreate          = "create"
	OpUpdate          = "update"
	OpDelete          = "delete"
	OpSet             = "set"
	OpAndSet          = "and_set"
)

// LogFileName is the on-disk filename of the audit log relative to
// the store root. Public so the doctor and audit subcommands can
// resolve the same path.
const LogFileName = ".thimble-audit.log"

// Event is one entry in the audit ledger. Operator is the K-27
// thumbprint (sha256(public-recipient)[:16] hex), never the
// recipient string itself; Subject is the secret key for secret ops
// or the recipient for recipient ops; values are NEVER recorded.
// Signers is populated only by recipient_add when a K-36 quorum
// gate produced the addition: it lists the operator thumbprints
// whose signatures were collected. Bootstrap is true when an add
// took the K-36 bootstrap path (≤1 existing recipient, no quorum
// gate active for the namespace).
type Event struct {
	Timestamp time.Time `json:"ts"`
	Operator  string    `json:"operator"`
	Op        string    `json:"op"`
	App       string    `json:"app"`
	Env       string    `json:"env"`
	Subject   string    `json:"subject,omitempty"`
	Signers   []string  `json:"signers,omitempty"`
	Bootstrap bool      `json:"bootstrap,omitempty"`
}

// Logger appends Events to a single audit log file. The zero value
// is unusable; construct one via New.
type Logger struct {
	path string
	warn io.Writer
}

// New returns a Logger that writes to <storeRoot>/.thimble-audit.log.
// warn receives a single line on append failure ("audit append
// failed: ...; mutation succeeded") and may be nil to silence. The
// ledger file is created lazily on first Append.
func New(storeRoot string, warn io.Writer) *Logger {
	return &Logger{
		path: filepath.Join(storeRoot, LogFileName),
		warn: warn,
	}
}

// Path returns the audit log file path.
func (l *Logger) Path() string { return l.path }

// Append writes event as a single JSON line to the audit log. The
// file is opened with O_APPEND|O_WRONLY|O_CREATE so concurrent
// writers do not interleave. On any IO failure the error is logged
// to the warn writer (so the operator sees it) and a non-nil error
// is returned to the caller — the caller is expected to log and
// continue, never abort the user's mutation (K-27 #3).
func (l *Logger) Append(event Event) error {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	line, err := json.Marshal(event)
	if err != nil {
		l.report(err)
		return err
	}
	line = append(line, '\n')
	// #nosec G304 -- l.path is the audit log inside the store root,
	// not user input at this layer.
	f, err := os.OpenFile(
		l.path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o640,
	)
	if err != nil {
		l.report(err)
		return err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		l.report(err)
		return err
	}
	if err := f.Close(); err != nil {
		l.report(err)
		return err
	}
	return nil
}

func (l *Logger) report(err error) {
	if l.warn == nil {
		return
	}
	fmt.Fprintf(l.warn, "audit append failed: %v; mutation succeeded\n", err)
}
