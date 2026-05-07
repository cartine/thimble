# K-01 Source-Code Size Standard Report

Date: 2026-05-07
Knot: thimble-16a2 ("Run /enforce-sourcecode-size")

## Standard

| Metric | Limit |
|--------|-------|
| File length | < 500 lines |
| Function/method body | < 100 lines |
| Line width | < 100 columns |

## Tooling installed

- [.golangci.yml](../../.golangci.yml) enables `funlen` (lines: 99) and
  `lll` (line-length: 100). `tasks/` and `.knots/` are excluded.
- [scripts/check_file_sizes.sh](../../scripts/check_file_sizes.sh) walks
  `*.go` files outside `vendor/`, `.knots/`, and `tasks/`, and fails on any
  file > 499 lines.
- [Makefile](../../Makefile) provides `make lint` which runs both checks.
- [CLAUDE.md](../../CLAUDE.md) and [AGENTS.md](../../AGENTS.md) document the
  standard at the repo root.

## Baseline violations (snapshot)

### File-length

| File | Lines | Status |
|------|------:|--------|
| `cmd/thimble/main.go` | 1,394 | OVER (will be fixed by [K-12](../knots/K-12-split-main-go.md)) |

### Function-length

None. Verified with a `go/ast` walker over `cmd/thimble/main.go` and
`cmd/thimble/main_test.go` — no function body exceeds 99 lines. The file
bulk is in the `uiTemplate` HTML/CSS string constant, not function bodies.

### Line-width (`lll`)

35 violations, all in `cmd/thimble/main.go`:

- 4 in Go source (lines 398, 585, 1064, and one signature). These are
  legitimate long expressions that should be wrapped during the normal
  K-12 split pass.
- 31 in the embedded `uiTemplate` HTML/CSS string constant (lines
  1229–1394). These are inside a Go string literal; the `lll` linter
  cannot tell. K-12 moves the template into its own file
  (`internal/web/template.html` or `template.go` with raw string blocks),
  which removes them from `lll`'s scope without changing rendered output.

## Verification

- `go build ./...` → exit 0.
- `go test ./...` → `ok cmd/thimble 1.775s`.
- `bash scripts/check_file_sizes.sh` → exits 1 on `main.go` (expected).
- `golangci-lint run` → exits 1 with the 35 lll findings (expected).
- `make lint` → composite of the two; exits 1 on the same set.

## Next

K-01 only ratifies the standard; no source modifications were made.
The expected lint failures will clear as part of:

- [K-12](../knots/K-12-split-main-go.md) — split `main.go` into packages
  (`internal/cli`, `internal/store`, `internal/age`, `internal/dotenv`,
  `internal/web`). Removes the file-size violation and the 31 template
  string violations (template moves out of Go source).
- The 4 remaining Go-source line-length violations get wrapped during
  K-12's review pass; if any survive, they're fixed in a follow-up commit
  on the K-12 branch.

The pre-commit hook is intentionally NOT installed by K-01. Per the
`/enforce-sourcecode-size` skill, the hook lands only after `make lint`
passes clean — so it lands after K-12.
