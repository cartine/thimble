# K-07 — Add CONTRIBUTING.md

- Wave / Step: 1.3
- Effort: S
- Risk: low
- Deps: —
- Files: CONTRIBUTING.md (new)

## Goal

Lower the bar for outside review. A CONTRIBUTING.md is itself a security
control: more eyeballs catch more issues earlier.

## Acceptance

CONTRIBUTING.md covers:
- Repo layout (post-K-12 package structure).
- How to run tests locally, including the integration job from K-16.
- The threat model (or pointer to README's threat-model section after K-09).
- How a release is cut (after K-48 lands, link to that target).
- Coding standards: size budgets from K-01, taxonomy from K-02.
- Required PR checks (after Wave 3 lands).

## Notes

Keep it under 200 lines. A long CONTRIBUTING.md is read by no one.
