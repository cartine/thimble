# K-35 — Startup banner: scope warning

- Wave / Step: 5.6
- Effort: S
- Risk: low
- Deps: K-12
- Files: internal/web/, internal/cli/

## Goal

Make the UI's intended scope explicit at startup so operators don't
mistakenly point a team or production deploy host at it.

## Acceptance

- `thimble web` prints, before the URL line, a 3-line block:
  ```
  Thimble web is a SINGLE-OPERATOR LOCAL TOOL.
  For shared/production workflows, use the CLI.
  Token rotates every 15m; press Ctrl+C to stop.
  ```
- The first page of the UI shows the same banner in muted styling (it's
  already half-there with the "values stay redacted" pill — extend it).

## Notes

This is a UX/documentation knot, not a code-control one — but it lowers the
chance of misuse, which is itself a security control.
