# K-36 — Quorum-signed recipient list

- Wave / Step: 6.1
- Effort: L
- Risk: high
- Deps: K-20, K-22
- Files: internal/store/, internal/recipient/, README.md, SECURITY.md

## Goal

Today, anyone with merge access to the consumer repo can add their own
recipient via a one-line manifest diff and unlock plaintext access to every
future write. The README's "verify out of band" rule is process, not
enforcement. Add a real gate.

## Acceptance

- Optional `secrets/recipients.signed.toml` file lists current operators and
  required quorum (e.g. M-of-N).
- `thimble recipient add <app> <env> <new>` requires either:
  - A signature file `secrets/.add-<sha>.sig` containing detached
    signatures (age signing or sigstore) from at least M of the N current
    operators, or
  - `--bootstrap` flag, valid only when the namespace has fewer than 2
    recipients (initial setup).
- `thimble recipient list <app> <env>` prints fingerprints + signature
  status.
- Tests: bootstrap path; M-of-N satisfied; M-of-N short-by-one rejected;
  signature over wrong recipient rejected.

## Notes

This is the structural fix for the single biggest social-engineering risk in
the model. Don't let it happen by accident; require it to be a deliberate
group action.
