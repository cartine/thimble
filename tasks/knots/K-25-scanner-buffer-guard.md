# K-25 — Scanner buffer guard for huge values

- Wave / Step: 4.8
- Effort: S
- Risk: low
- Deps: K-12
- Files: internal/dotenv/

## Goal

`bufio.NewScanner` defaults to a 64 KiB token. A secret value larger than
that fails to parse with `bufio.ErrTooLong` — message is unhelpful, and
operators have no idea their secret was silently truncated on the way in.

## Acceptance

- Dotenv parser explicitly sets `scanner.Buffer` to a sensible max
  (e.g. 1 MiB).
- Beyond that limit, parser returns "value on line N exceeds <limit> bytes;
  store it as a file or split it."
- Test for: 64 KiB+1 value round-trips fine; 1 MiB+1 value fails with the
  clear message.

## Notes

Don't go infinite — a truly enormous value pegs memory and is almost
certainly a mistake (e.g. someone piping a binary blob).
