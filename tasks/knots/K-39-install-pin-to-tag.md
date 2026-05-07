# K-39 — install.sh: pin to tag, not main

- Wave / Step: 7.2
- Effort: S
- Risk: med
- Deps: K-38
- Files: scripts/install.sh, README.md

## Goal

The README's quickstart points at
`raw.githubusercontent.com/cartine/thimble/main/scripts/install.sh`. Anyone
with push access to `main` can change the install script for everyone in
flight. Pin the URL.

## Acceptance

- README quickstart's `curl … install.sh | sh` URL points at a tagged
  commit, e.g. `…/raw/v0.1.0/scripts/install.sh`.
- The release workflow uploads `install.sh` itself as a release asset; the
  README also documents `curl …/releases/latest/download/install.sh | sh`
  as an alternative.
- The script's first line prints its own SHA-256 (computed at build time)
  so an operator can spot-check.

## Notes

Cheap. Do at the same time as K-38 to avoid two README-edit PRs.
