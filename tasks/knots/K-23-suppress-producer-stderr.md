# K-23 — Suppress producer stderr by default

- Wave / Step: 4.6
- Effort: S
- Risk: med
- Deps: K-12
- Files: internal/cli/, README.md

## Goal

`runSecretProducer` mirrors child stderr to the parent
([cmd/thimble/main.go:884](cmd/thimble/main.go:884)). Producers that log to
stderr — and worse, ones running under `set -x` or with debug flags — leak
the secret straight to the operator's terminal.

## Acceptance

- Default behavior: producer stderr is captured into a buffer used only for
  the error message on non-zero exit.
- New `--show-stderr` flag opts back in to live mirroring for debugging.
- On success, captured stderr is discarded.
- On failure, captured stderr is run through `redact` (truncate to 240
  chars) before being shown.
- Test covers: producer that writes the secret to stderr does not leak it
  to the parent's stderr in the success case.

## Notes

The same logic should be applied to `runSecretConsumer` if it ever becomes
chatty; currently it inherits the parent's stderr by design (the child *is*
the user's command).
