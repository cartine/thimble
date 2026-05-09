# K-41 — Reproducible-build verify target

- Wave / Step: 7.4
- Effort: M
- Risk: low
- Deps: K-40
- Files: Makefile, scripts/verify-release.sh, README.md

## Goal

Document and prove that builds are deterministic. Provide a one-liner that
rebuilds locally and compares to the released checksum.

## Acceptance

- `make verify-release VERSION=v0.1.0` (or equivalent) checks out the tag,
  rebuilds with the same `-trimpath -ldflags="-s -w"` flags and CGO/GOOS
  matrix, and diffs the resulting tarball SHA-256s against the published
  `checksums.txt`.
- README "Verifying releases" section walks through the steps.
- CI gains a "reproducibility" job (optional, slow) that runs the same
  verification monthly or on release.

## Notes

Reproducible Go builds are mostly a function of `-trimpath` (already on),
matched Go version (`go-version-file`), and stable third-party deps
(already minimal). This knot is mostly about *demonstrating* the property,
not building it.
