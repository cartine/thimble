# K-02 — Run /taxonomize

- Wave / Step: 0.2
- Effort: S
- Risk: low
- Deps: K-01
- Files: TAXONOMY.md (new)

## Goal

Extract the canonical Thimble vocabulary — `app`, `environment`, `namespace`,
`recipient`, `bundle`, `manifest`, `identity`, `render`, `provision`, `set`,
`and-set`, `and-get`, `peer`, `operator`, `deploy host`, `recovery recipient`
— into a single `TAXONOMY.md`. Surface terminology drift between
[README.md](README.md), [SECURITY.md](SECURITY.md), [thimble.md](thimble.md),
and [cmd/thimble/main.go](cmd/thimble/main.go) so later waves can resolve it.

## Acceptance

- `TAXONOMY.md` exists at repo root with: nouns, verbs, phrases, and
  per-term definition + canonical synonym (e.g. "namespace = `<app>/<env>`").
- The skill's drift report is committed at `tasks/reports/K-02-taxonomy.md`
  listing every term that is used inconsistently across docs and code.
- No source/doc edits yet; this knot only catalogs.

## Notes

Thimble overloads `set` (CLI verb), `set` (Go method), and "set" as in "the
secret has been set" — that's exactly the kind of ambiguity TAXONOMY.md should
freeze. Same with `environment` (deploy context) vs `env` (manifest field) vs
`env` (CLI argument).
