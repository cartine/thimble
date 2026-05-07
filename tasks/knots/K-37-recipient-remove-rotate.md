# K-37 — `recipient remove --rotate` flow

- Wave / Step: 6.2
- Effort: M
- Risk: med
- Deps: K-36
- Files: internal/cli/, internal/store/, internal/age/

## Goal

Removing a recipient does not invalidate plaintext or encrypted copies the
former peer already obtained. SECURITY.md acknowledges this. UX-wise, the
right move is to make rotation the easy default after a removal.

## Acceptance

- `thimble recipient remove <app> <env> <recipient> --rotate` regenerates
  every value created via `provision` (those marked random in metadata) and
  prompts for replacements on each non-random value.
- `--rotate-randoms-only` skips interactive prompts and only rotates the
  high-entropy generated keys.
- A summary at the end lists keys rotated, keys skipped, and keys that
  still need attention out of band ("STRIPE_KEY: rotate via Stripe
  dashboard").
- Tests: provisioned key gets a fresh value; user-supplied key prompts
  correctly; cancel mid-flow leaves manifest in a consistent state (uses
  K-21 locks).

## Notes

Mark a value as "provisioned" in the manifest when it's set via `provision`
output (or via `and-set` from `provision`). That metadata is what
`--rotate-randoms` uses; it never touches values whose origin we can't
infer.
