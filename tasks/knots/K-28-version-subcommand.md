# K-28 — `thimble --version` subcommand

- Wave / Step: 4.11
- Effort: S
- Risk: low
- Deps: K-12
- Files: internal/cli/, .github/workflows/release.yml

## Goal

Operators can't tell what binary is on a host vs. what shipped a fix without
this. Bare-minimum operability.

## Acceptance

- `thimble --version` and `thimble version` both print: `thimble vX.Y.Z
  (commit abc1234, built 2026-05-07T14:00:00Z, go1.25.0)`.
- Release workflow injects version/commit/date via `-ldflags="-X
  main.version=…"` etc.
- Local builds without ldflags print `dev` for version and the git short SHA
  if available, otherwise `unknown`.

## Notes

Cheap, high-value, frequently the first thing on an operator's checklist.
