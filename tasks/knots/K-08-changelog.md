# K-08 — Add CHANGELOG.md (Keep-a-Changelog)

- Wave / Step: 1.4
- Effort: S
- Risk: low
- Deps: —
- Files: CHANGELOG.md (new)

## Goal

For a tool whose pitch is "trust me with your prod secrets," the auditor's
first read is "what changed and why." Adopt Keep-a-Changelog and Semantic
Versioning conventions explicitly.

## Acceptance

- `CHANGELOG.md` exists with `[Unreleased]` and a backfilled `[0.1.0]` entry
  capturing the current state.
- README's install snippet pins to a real tag (see K-11).
- Future release workflow (K-40, K-48) generates entries from PR titles or
  conventional-commits scopes.

## Notes

Don't try to backfill history in detail — the repo only has 2 commits. A
single "initial release" bullet under 0.1.0 is fine.
