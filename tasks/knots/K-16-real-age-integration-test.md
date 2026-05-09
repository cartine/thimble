# K-16 — Integration test against real `age` binary

- Wave / Step: 3.4
- Effort: M
- Risk: med
- Deps: K-13
- Files: .github/workflows/ci.yml, internal/age/*_integration_test.go (new)

## Goal

Today's tests substitute a sed-based fake `age` binary
([cmd/thimble/main_test.go:266](cmd/thimble/main_test.go:266)). That proves
arg-construction round-trips, not that the real protocol works. A real
integration test catches drift if `age` flags or output format change.

## Acceptance

- A new integration test job in CI installs `age` (apt or release tarball),
  generates an identity, runs the full lifecycle: init → set → list → render
  → recipient add → recipient remove, and asserts the rendered plaintext
  matches what was set.
- Tests are tagged `//go:build integration` so they don't slow the unit-test
  job.
- Job runs on at least Ubuntu; ideally macOS too.

## Notes

This is the test that would have caught a hostile `age` swap at install time
— close to but not the same as K-18 (which addresses the runtime-side trust
boundary).
