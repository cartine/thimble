# K-29 — `thimble doctor` subcommand

- Wave / Step: 4.12
- Effort: M
- Risk: low
- Deps: K-18, K-19, K-21, K-28
- Files: internal/cli/, internal/doctor/

## Goal

A one-shot health/setup check that surfaces every common pitfall before the
operator hits it in production.

## Acceptance

`thimble doctor` checks and reports each:
- `age` resolved path + version + SHA-256.
- Identity file: presence, mode (0600 expected), readable.
- Store directory: presence, mode (0700 expected), writeability.
- Manifest: parseable, no version conflicts, all referenced bundles exist.
- Web UI port (`127.0.0.1:8787` by default): availability.
- Recipient list per namespace: count, format (after K-20), thumbprints.

Output is tabular; non-zero exit on any failure. Acceptance includes a test
that constructs a deliberately-broken setup and asserts each diagnostic
fires.

## Notes

This is the knot that's worth pointing CONTRIBUTING.md at: anyone setting
up locally runs `thimble doctor` and gets a list of fixes.
