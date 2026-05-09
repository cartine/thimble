# K-20 — Strict recipient format validation

- Wave / Step: 4.3
- Effort: S
- Risk: med
- Deps: K-12
- Files: internal/age/, cmd/thimble/main_test.go

## Goal

`validateRecipient` only checks for whitespace and emptiness. A typo
(`age0…` instead of `age1…`) is accepted at init/add time and only fails at
the next decrypt with a confusing `age` error — sometimes hours later.

## Acceptance

- `validateRecipient` accepts only:
  - `age1` followed by a Bech32 charset of expected length (X25519 recipients).
  - SSH recipients beginning with `ssh-ed25519 ` or `ssh-rsa ` followed by
    a base64 blob and optional comment.
- Anything else fails with a precise message: "expected `age1…` or
  `ssh-ed25519 …`/`ssh-rsa …`; got `<truncated input>`."
- Tests cover: valid age1, valid ssh-ed25519, malformed age0 (rejected),
  random string (rejected), trailing newline trimmed, leading dash rejected.

## Notes

Tightening this also reduces the social-engineering surface on K-36's
recipient-quorum scheme: the simpler the format, the easier to eyeball in a
diff.
