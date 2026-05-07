# K-11 — Pin README install example to a real tag

- Wave / Step: 1.7
- Effort: S
- Risk: low
- Deps: —
- Files: README.md

## Goal

The README's `THIMBLE_VERSION=v0.1.0` references a tag that does not exist in
this repo. Either ship `v0.1.0` (preferred) or use a `vX.Y.Z` placeholder.

## Acceptance

- Either:
  - Tag `v0.1.0` exists, the release pipeline produced its artifacts, and the
    README example continues to use `v0.1.0`; OR
  - The README example uses `vX.Y.Z` with a sentence pointing readers at the
    "latest release" page.
- README's "latest" curl-pipe-sh example still works — verified by running it
  in a clean container.

## Notes

If pursuing the first option, this knot is gated on K-08 (CHANGELOG) and the
release workflow already in place.
