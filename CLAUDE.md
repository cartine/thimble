# Thimble — Project Guidance

Thimble is a small, file-first secrets manager wrapping `age`. Code in this
repo MUST stay auditable: small files, small functions, no clever indirection.

## Source Code Size Standard

Run `make lint` before committing. All source files must stay within:
`<500` lines/file, `<100` lines/function, and `<100` columns/line.

The standard is enforced by `.golangci.yml` (funlen + lll) and
`scripts/check_file_sizes.sh`.

## Vocabulary

See [TAXONOMY.md](TAXONOMY.md) for canonical terms (application,
environment, namespace, recipient, identity, bundle, ...). Update via
`/taxonomize`.

## Workflow

This repo uses `kno` for execution-plan tracking. See `tasks/knot-plan.md`
for the active rollout. Run `kno ls` for the live state.

## Build

`go build ./cmd/thimble` from a checkout. Wider targets (test, integration,
vuln, release) land in [K-47](tasks/knots/K-47-makefile.md).
