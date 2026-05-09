# K-27 — Append-only audit log

- Wave / Step: 4.10
- Effort: M
- Risk: low
- Deps: K-21
- Files: internal/store/, internal/audit/, README.md

## Goal

SECURITY.md acknowledges the gap: no audit log. Even a local, file-based
ledger of `{when, by, op, app, env, key}` would make incident response and
operator handoffs dramatically easier.

## Acceptance

- `secrets/.thimble-audit.log` is appended on every mutating op
  (init, recipient add/remove, create, update, delete, set, and-set).
- Each entry: timestamp (UTC), operator identifier (recipient thumbprint of
  the identity used to decrypt this session — stable, non-secret), op,
  app, env, key (or recipient).
- File mode 0640.
- `thimble audit <app> <env>` subcommand pretty-prints the log filtered by
  namespace.
- Audit writes never block on IO failure; on failure they emit a stderr
  warning but do not abort the user's mutation.

## Notes

Don't include values, ever. Don't include identity file paths. Thumbprints
are public — they can sit alongside encrypted bundles in any store without
harm.
