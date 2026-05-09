# K-26 — Timeout + cancellable context for `age` subprocess

- Wave / Step: 4.9
- Effort: S
- Risk: low
- Deps: K-12
- Files: internal/age/

## Goal

`exec.CommandContext(context.Background(), …)` never cancels. A stuck or
hostile `age` hangs the operator's CLI indefinitely — the only signal is
"this is taking forever."

## Acceptance

- All `age` invocations use a timeout context: 10 s default, configurable
  via `THIMBLE_AGE_TIMEOUT` (in seconds).
- Signal handling: SIGINT to Thimble cancels the child cleanly.
- On timeout, error message is "age timed out after Ns; rerun with
  `THIMBLE_AGE_TIMEOUT=N` if your hardware is slow or the bundle is large."

## Notes

10 s is generous for typical bundles. Large bundles (tens of MB) might need
longer; the env var is the escape valve.
