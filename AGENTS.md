# Agent Guidance for Thimble

This repo uses [knots](https://github.com/cartine/knots) (`kno`) for
execution-plan tracking. The 48-knot Thimble hardening plan is in
`tasks/knot-plan.md`; individual drafts are in `tasks/knots/K-NN-*.md`.

When picking up work:

1. `kno ls` — see active knots.
2. `kno show <id>` — read context.
3. `kno claim <id>` — start work; the claim output prescribes the per-state
   actions.
4. `kno next <id> --expected-state <s> --lease <lease>` — advance.
5. `kno rollback <id>` — abort cleanly if the state's goals were not met.

## Source Code Size Standard

All source files under tracked directories must satisfy:

| Metric | Limit |
|--------|-------|
| File length | < 500 lines |
| Function/method body | < 100 lines |
| Line width | < 100 columns |

Enforcement: run `make lint` before merge. It must pass `golangci-lint`
(with `funlen` and `lll` enabled) and `scripts/check_file_sizes.sh`.
