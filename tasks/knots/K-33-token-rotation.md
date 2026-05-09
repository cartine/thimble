# K-33 — Web token rotation (`--rotate-token`)

- Wave / Step: 5.4
- Effort: S
- Risk: low
- Deps: K-30
- Files: internal/web/

## Goal

Today's token lives for the lifetime of the server process and never
rotates. Add manual rotation, plus an idle-rotate timer.

## Acceptance

- `thimble web rotate-token` (or a SIGUSR1 handler if simpler) regenerates
  the token, prints the new value to the controlling terminal, invalidates
  all existing sessions.
- `--idle-rotate=15m` (default) rotates the token after N minutes of no
  authenticated requests; existing sessions then re-authenticate.
- Tests cover: post-rotation, old cookies are 401; new token works; SIGUSR1
  handling on platforms that support it.

## Notes

Idle rotation is the cheap win. Manual rotation is the operator's escape
hatch when they think a token may have leaked.
