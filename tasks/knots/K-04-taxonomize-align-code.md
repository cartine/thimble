# K-04 — Run /taxonomize-align-code

- Wave / Step: 0.4
- Effort: M
- Risk: med
- Deps: K-03
- Files: cmd/thimble/main.go, cmd/thimble/main_test.go

## Goal

Align Go identifiers (types, functions, vars, fields, comments, error
messages) to TAXONOMY.md. Produces a rename plan, gates on review, then
executes language-aware refactors.

## Acceptance

- A rename plan is committed at `tasks/reports/K-04-rename-plan.md` and
  approved before any rename runs.
- After execution, all type names match canonical taxonomy:
  `manifest`, `appManifest`, `envManifest`, `store`, `secretEntry`,
  `namespaceView`, `webServer` — confirm or rename per TAXONOMY.md.
- All exported and package-internal identifiers in main.go are consistent
  with TAXONOMY.md (e.g. if `namespace` is canonical, `appEnv` becomes
  `namespace`).
- `go build ./...` and `go test ./...` both pass with no behavioral change.
- Public CLI surface is unchanged: `thimble init`, `set`, `render`, etc. —
  user-visible verbs are sticky and only renamed if TAXONOMY.md explicitly
  marks them as drift.

## Notes

This is the last knot in Wave 0. After it lands, Wave 2's package split has
clean naming to organize against. Do not allow this knot to touch CLI verbs or
flag names without a paired README/CHANGELOG entry.
