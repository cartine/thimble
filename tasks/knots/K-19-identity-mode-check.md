# K-19 — Identity-file mode sanity check

- Wave / Step: 4.2
- Effort: S
- Risk: med
- Deps: K-12
- Files: internal/age/

## Goal

Refuse to use a `THIMBLE_AGE_IDENTITY` file that is group/world readable.
Catches the most common operator mistake on the most sensitive file Thimble
touches.

## Acceptance

- Before invoking `age -i`, Thimble `Stat`s the identity file.
- If `mode & 0o077 != 0`, abort with: "identity file <path> is mode 0NNN;
  expected 0600. Run `chmod 0600 <path>` and retry."
- `--unsafe-allow-identity-mode` flag exists for environments where mode
  cannot be set (e.g. some Windows / WSL setups), and using it logs a warning
  to stderr.
- Test covers: 0600 OK, 0640 rejected, flag overrides rejection.

## Notes

`age-keygen -o` defaults to 0600, so this is mostly defending against people
copying identities around with `cp`/`scp`/`mv` and dropping permissions.
