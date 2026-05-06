# Security Review

This implementation is approved for the intended small-team, file-first Thimble
slice when used with `age` and normal operator hygiene.

## Approved Controls

- No custom cryptography. Thimble shells out to the audited `age` binary.
- Secrets are scoped by application and environment.
- Plaintext is kept in memory for the active command and is not written to a
  working-tree temp file.
- Encrypted bundles and metadata are written with atomic rename.
- Files are created with restrictive modes: store directories `0700`, files
  `0600`.
- Listing and web UI views expose keys and metadata only, not values.
- Web UI requires a token. Non-loopback binds are rejected unless a token is
  explicitly supplied.
- Secret names, app names, environment names, and recipients are validated before
  touching the filesystem or invoking `age`.
- Recipient changes re-encrypt the bundle so metadata and ciphertext remain in
  sync.

## Residual Risks

- Command-line values can be captured by shell history or process listings. For
  sensitive entry, pipe values through stdin.
- The web UI is an operator tool, not a multi-user hosted service. Use it on
  loopback or behind a trusted tunnel.
- Decryption requires an authorized age identity. A compromised operator machine
  or deploy host can read any secret that identity can decrypt.
- This does not yet provide audit logs, policy approval workflows, or automatic
  rotation.

## Security Agent Verdict

Approved for the requested implementation slice: basic CRUD tooling,
application/environment namespaces, a local web UI, and release-script install
and update. The implementation follows the `thimble.md` non-goal of not
implementing cryptography and keeps remaining risks explicit.
