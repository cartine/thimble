# K-48 — `make tag-release` automation

- Wave / Step: 8.3
- Effort: M
- Risk: med (touches the release pipeline)
- Deps: K-08, K-40, K-41, K-47
- Files: Makefile, scripts/tag-release.sh

## Goal

Releases-as-a-button. Today, cutting a release means pushing a tag and
hoping the workflow is healthy. Wrap that into one target that's safe to
run.

## Acceptance

- `make tag-release VERSION=v0.1.1` does:
  1. Refuses if working tree is dirty or branch != `main`.
  2. Refuses if `CHANGELOG.md` does not have an entry for `VERSION`.
  3. Tags and pushes.
  4. Watches the release workflow via `gh run watch`.
  5. After workflow success, downloads each artifact, verifies checksum
     against published `checksums.txt`, runs `gh attestation verify`
     (K-40).
  6. Prints a single "ready" line with the release URL on success, or a
     diagnostic on failure.
- Tests / dry runs: `--dry-run` flag prints the steps without acting.

## Notes

This is the closing knot of the plan. After it lands, every step from
"I want to release" to "I can recommend operators upgrade" is reproducible
and observable.
