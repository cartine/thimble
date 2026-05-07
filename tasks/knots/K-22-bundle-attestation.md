# K-22 — Bundle attestation (ciphertext SHA + signed manifest)

- Wave / Step: 4.5
- Effort: L
- Risk: med
- Deps: K-21
- Files: internal/store/, internal/age/, README.md

## Goal

A repo writer can swap `production.env.age` for an older bundle, or one
encrypted only to themselves, while leaving the manifest's recipient list
unchanged. Downstream peers won't notice. Bind the manifest to the
ciphertext.

## Acceptance

- Manifest stores `bundle_sha256` for each `envManifest`. On decrypt/render,
  Thimble recomputes and rejects if mismatched.
- Optional: manifest is signed by one operator's age identity (or a sigstore
  signature) covering all `bundle_sha256`s and the recipient list. Verified
  before any write proceeds.
- `thimble verify <app> <env>` subcommand prints the bundle SHA, the
  manifest signature status, and the recipient list with thumbprints.
- README "Peer Safety Rules" gains a bullet about reviewing
  `bundle_sha256` changes alongside ciphertext changes.

## Notes

Phase this: ship the SHA-in-manifest first (low effort, big win). Add the
signed manifest later — that piece is L on its own and depends on a key
management story we don't yet have.
