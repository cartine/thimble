# Security Review (Internal)

> Snapshot of an internal review. Not the disclosure policy — see
> [SECURITY.md](../SECURITY.md) for how to report a vulnerability.
> Reviewer: Andrew Cartine. Date of review: 2026-05-06.

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
- Secret values are rejected when supplied as command arguments.
- CLI create, update, and set read from a pipe or a masked terminal prompt.
- `provision` refuses weak generated values and is designed for piping into
  storage flows.
- `and-set` captures a command's stdout and stores it without echoing the value.
- `and-get` passes a value to a child command on stdin by default; environment
  variable exposure is explicit with `--env`.
- Web UI requires a token. Non-loopback binds are rejected unless a token is
  explicitly supplied.
- Secret names, app names, environment names, and recipients are validated before
  touching the filesystem or invoking `age`.
- Recipient changes re-encrypt the bundle so metadata and ciphertext remain in
  sync.
- Peer setup is recipient-based: peers exchange encrypted bundles and verified
  public recipients, never private age identities.

## Residual Risks

- Explicit `and-get --env` use can expose values through child process
  environments. Prefer stdin where tools allow it.
- The web UI is an operator tool, not a multi-user hosted service. Use it on
  loopback or behind a trusted tunnel.
- Decryption requires an authorized age identity. A compromised operator machine
  or deploy host can read any secret that identity can decrypt.
- Removing a recipient does not invalidate plaintext or encrypted copies they
  already obtained. Rotate high-risk values after access removal.
- This does not yet provide audit logs, policy approval workflows, or automatic
  rotation.

## Reviewer Notes

This review approves the requested implementation slice: basic CRUD tooling,
application/environment namespaces, peer-capable encrypted bundle sync, safe
secret entry, a local web UI, and release-script install and update. The
implementation follows the `thimble.md` non-goal of not implementing
cryptography and keeps remaining risks explicit.

The active hardening rollout addressing the residual risks is tracked in
[../tasks/knot-plan.md](../tasks/knot-plan.md).
