package store

import (
	"sync"

	"github.com/cartine/thimble/internal/audit"
)

// Audit op aliases so store callers do not need to import the audit
// package just to name the operation. Kept in sync with
// internal/audit/audit.go.
const (
	auditOpInit            = audit.OpInit
	auditOpRecipientAdd    = audit.OpRecipientAdd
	auditOpRecipientRemove = audit.OpRecipientRemove
	auditOpCreate          = audit.OpCreate
	auditOpUpdate          = audit.OpUpdate
	auditOpDelete          = audit.OpDelete
	auditOpSet             = audit.OpSet
)

// recipientThumbprint returns audit.Thumbprint(recipient). Wrapped
// here so the call sites in store.go stay short.
func recipientThumbprint(recipient string) string {
	return audit.Thumbprint(recipient)
}

// auditState holds the K-27 audit logger and the lazily computed
// operator thumbprint for the lifetime of a Store. Stored separately
// from Store fields to keep store.go small; SetAuditLogger installs
// it.
type auditState struct {
	logger    *audit.Logger
	once      sync.Once
	thumb     string
	thumbErr  error
	identityP func() string
}

// SetAuditLogger installs the K-27 audit logger. The identity-path
// closure lets the store resolve the operator thumbprint from the
// configured identity file lazily on first event. Pass a nil logger
// to disable auditing (the test harness defaults to nil).
func (s *Store) SetAuditLogger(logger *audit.Logger) {
	if logger == nil {
		s.audit = nil
		return
	}
	s.audit = &auditState{
		logger:    logger,
		identityP: func() string { return s.age.Identity() },
	}
}

// operatorThumbprint returns the cached operator thumbprint, or
// audit.UnknownOperator if no identity is configured or the file is
// unreadable. Computed once per Store lifetime under sync.Once.
func (s *Store) operatorThumbprint() string {
	if s.audit == nil {
		return audit.UnknownOperator
	}
	s.audit.once.Do(func() {
		path := ""
		if s.audit.identityP != nil {
			path = s.audit.identityP()
		}
		if path == "" {
			s.audit.thumb = audit.UnknownOperator
			return
		}
		recipient, err := audit.PublicRecipientFromIdentityFile(path)
		if err != nil {
			s.audit.thumb = audit.UnknownOperator
			s.audit.thumbErr = err
			return
		}
		s.audit.thumb = audit.Thumbprint(recipient)
	})
	if s.audit.thumb == "" {
		return audit.UnknownOperator
	}
	return s.audit.thumb
}

// recordEvent appends a single audit event for op against (app, env)
// with optional subject (key for secret ops, recipient thumbprint
// for recipient ops). Audit IO failure is reported through the
// logger's warn writer and the function returns nil so the caller
// never aborts the user's mutation (K-27 #3).
func (s *Store) recordEvent(op, app, env, subject string) {
	s.recordEventWithSigners(op, app, env, subject, nil, false)
}

// recordEventWithSigners is the K-36 variant that captures the
// signing operator thumbprints (for recipient_add) and the bootstrap
// flag. Other ops should keep using recordEvent; the extra fields
// are omitempty so the JSON line stays short.
func (s *Store) recordEventWithSigners(
	op, app, env, subject string, signers []string, bootstrap bool,
) {
	if s.audit == nil || s.audit.logger == nil {
		return
	}
	_ = s.audit.logger.Append(audit.Event{
		Operator:  s.operatorThumbprint(),
		Op:        op,
		App:       app,
		Env:       env,
		Subject:   subject,
		Signers:   signers,
		Bootstrap: bootstrap,
	})
}
