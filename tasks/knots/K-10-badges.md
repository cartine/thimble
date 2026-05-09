# K-10 — Add status badges to README

- Wave / Step: 1.6
- Effort: S
- Risk: low
- Deps: K-13 (so the build badge has something to point at)
- Files: README.md

## Goal

Cheap signal that the project is alive. Helps a reader within 2 seconds.

## Acceptance

- README header shows badges for: build (Wave 3 CI), latest release version,
  Go version (from `go.mod`), license (from K-05).
- All badges link to authoritative sources (Actions tab, releases page,
  pkg.go.dev, OSI license page).
- No vanity badges (no "made with love", no Discord/Slack invite unless they
  exist).

## Notes

K-13 must land before this knot's build badge resolves to a real workflow.
Other badges (license, Go version) work immediately.
