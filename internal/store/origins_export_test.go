package store

import "errors"

// ArmFailNextOriginsSave is the test-only entry point that arms the
// fault-injection hook so the next call to saveOriginsWithHook
// returns an error. K-37 tests use this to verify the rotation flow
// rolls the bundle and origins file back to their pre-rotation
// state when the post-encrypt save of origins.json fails.
func (s *Store) ArmFailNextOriginsSave() {
	s.faults.failNextOriginsSave = true
}

// IsFaultInjectionOriginsSaveError reports whether err wraps the
// synthetic error returned by the fault-injection hook. Used by
// tests to assert the failure path returned the expected sentinel
// rather than an unrelated error.
func IsFaultInjectionOriginsSaveError(err error) bool {
	return errors.Is(err, errFaultInjectionOriginsSave)
}
