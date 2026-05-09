# K-01 — Run /enforce-sourcecode-size

- Wave / Step: 0.1
- Effort: S
- Risk: low
- Deps: —
- Files: cmd/thimble/main.go, cmd/thimble/main_test.go, any new size-budget config

## Goal

Establish file/function/line-length budgets for the repo before any other work
begins. Today `cmd/thimble/main.go` is 1,394 lines in a single file; that's the
strongest signal that there are no agreed limits. Run `/enforce-sourcecode-size`
to produce concrete budgets and a delta report.

## Acceptance

- A budget file (e.g. `.sourcecode-size.toml` or whatever the skill emits)
  is committed at repo root.
- The skill's report is captured in `tasks/reports/K-01-enforce.md` and lists
  each over-budget file/function with its measured size.
- README "Contributing" pointer (or CONTRIBUTING.md after K-07) references the
  budget file.
- Repository builds and tests still pass; no source modifications yet.

## Notes

This knot only measures and ratifies budgets. It does not split files — that's
K-12. Treat the budget as the contract Wave 2 must satisfy.
