# K-30 — Move web token from URL to cookie

- Wave / Step: 5.1
- Effort: M
- Risk: med
- Deps: K-12
- Files: internal/web/

## Goal

The startup line prints `http://127.0.0.1:8787/?token=…`. That URL ends up in
shell history, terminal scrollback, screen-share recordings, and (for
non-loopback binds) browser history. A cookie scoped properly fixes it.

## Acceptance

- New flow: server prints the token at startup. The first visit to `/`
  without a session cookie shows a one-field form ("paste token to log in").
- On submit, server compares with `subtle.ConstantTimeCompare`, sets a
  `thimble_session` cookie with `HttpOnly; Secure (when not loopback);
  SameSite=Strict; Path=/; Max-Age=3600`, and redirects to `/`.
- All authenticated routes accept the cookie; the `?token=…` form is removed
  from links.
- Logout link clears the cookie.
- Tests: missing cookie → 401; wrong token → 401; correct token → 303 +
  Set-Cookie; subsequent requests with cookie → 200.

## Notes

For loopback-only binds, `Secure` is omitted (browsers reject it on HTTP).
For non-loopback binds (which already require `--token`), require HTTPS or
print a warning.
