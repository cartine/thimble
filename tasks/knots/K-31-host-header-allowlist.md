# K-31 — Host-header allowlist (DNS rebinding defense)

- Wave / Step: 5.2
- Effort: S
- Risk: med
- Deps: K-12
- Files: internal/web/

## Goal

The loopback guard checks the listen address, not the incoming `Host` header.
A malicious page that DNS-rebinds a hostname to `127.0.0.1` would still hit
the server; only the 256-bit token saves you. Add explicit Host validation.

## Acceptance

- Middleware rejects requests whose `Host` header is not in the allowlist:
  `127.0.0.1[:port]`, `[::1][:port]`, `localhost[:port]` (plus any
  user-supplied address via `--addr`).
- Rejection is `400 Bad Request` with body "host not allowed".
- For non-loopback binds, allowlist defaults to the configured `--addr`'s
  host; a `--allow-host` flag lets the operator add more.
- Tests: matching Host → 200; foreign Host → 400; case differences in
  `Host` handled.

## Notes

This closes the only remaining DNS-rebinding vector worth worrying about for
a local-only tool. Combined with K-30 (cookies + SameSite=Strict) it's
defense in depth, not single-control.
