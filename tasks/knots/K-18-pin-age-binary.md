# K-18 — Pin/verify `age` binary path

- Wave / Step: 4.1
- Effort: M
- Risk: high (single biggest runtime trust gap)
- Deps: K-12
- Files: internal/age/, internal/cli/, README.md, SECURITY.md

## Goal

`s.agePath = "age"` resolves through `$PATH`. A malicious binary earlier in
the path silently captures plaintext during encrypt and the identity-file
path during decrypt — every Thimble user's secrets, on that machine, with
zero indication.

## Acceptance

- `--age-binary=/path/to/age` flag and `THIMBLE_AGE_BINARY` env var supported.
- On startup, Thimble resolves the binary path via `exec.LookPath` and prints
  it once on `--verbose` or first-use of any decrypt/encrypt command.
- Optional SHA-256 pin: `THIMBLE_AGE_SHA256` is checked before exec; mismatch
  aborts with a clear error.
- README "Requirements" section is updated to call this out as a trust
  boundary; SECURITY.md threat model lists it as a residual risk if pinning
  is not used.
- `thimble doctor` (K-29) reports the resolved path and SHA-256.

## Notes

Don't make the SHA pin mandatory — that breaks installs across distros. Make
it loud-default and trivial to enable.
