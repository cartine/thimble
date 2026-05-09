# K-34 — Plaintext-input policy on web UI

- Wave / Step: 5.5
- Effort: M
- Risk: med
- Deps: K-12
- Files: internal/web/

## Goal

The web UI's `<input type="password">` accepts plaintext in form bodies — a
direct contradiction of the otherwise-strict CLI rule of never accepting
values via path-visible mechanisms. Form bodies live in browser autofill,
page-cache, refresh-resubmit, DevTools network panel.

## Acceptance

Pick one (or both gated by config):

- **Strict mode (default)**: Web UI removes the value field entirely. The UI
  becomes purely a *redacted* viewer + recipient/key manager. To set a
  value, the operator must run a CLI command. The page shows the exact
  shell command to copy: `thimble set <app> <env> <KEY>`.
- **Convenience mode**: Web UI keeps the value field but adds a clearly
  styled "low-risk only" banner, sets `autocomplete="off"` and
  `data-1p-ignore` (1Password), and never echoes the value back even on
  validation failure.

Acceptance:
- The choice is documented in README and SECURITY.md.
- Tests confirm: in strict mode, POST to `/secret` with action `create` or
  `update` and a non-empty `value` returns 400 with a pointer at the CLI.

## Notes

Strong default recommendation: strict mode. The UI's job is "let me see
namespaces and manage recipients," not "type secrets into a textarea."
