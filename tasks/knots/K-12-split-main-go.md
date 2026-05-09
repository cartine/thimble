# K-12 — Split main.go into packages

- Wave / Step: 2.1
- Effort: M
- Risk: med
- Deps: K-01, K-04
- Files: cmd/thimble/main.go (shrink to entrypoint), internal/*

## Goal

`cmd/thimble/main.go` is 1,394 lines covering CLI parsing, store CRUD, age
shelling, dotenv parse/encode, web server, and HTML template. For a secrets
tool, auditability is a feature; a reviewer should land in a 200-line file
and answer "does this leak?" quickly.

## Acceptance

Proposed package layout:
- `cmd/thimble/main.go` — entrypoint only, < 100 lines, just dispatches to
  `internal/cli`.
- `internal/cli/` — flag parsing, subcommand dispatch.
- `internal/store/` — manifest + envManifest + atomic writes.
- `internal/age/` — exec wrapper for `age` (encrypt, decrypt).
- `internal/dotenv/` — parse, encode, quoting.
- `internal/web/` — server, routes, template.

Acceptance:
- All packages respect K-01 size budgets.
- All identifiers respect TAXONOMY.md (K-02/K-04).
- `go test ./...` passes; behavior unchanged.
- Public CLI surface unchanged.
- Each package has a 1–3 sentence package doc comment explaining its trust
  boundary (e.g. "internal/age is the only package that handles plaintext
  outside of memory mapped within a single command").

## Notes

This is mechanical once Wave 0 is settled. The work is bigger if Wave 0 is
skipped — names and shapes will churn.
