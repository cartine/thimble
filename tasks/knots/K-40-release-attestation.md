# K-40 — Release: sigstore attestation + GH attest

- Wave / Step: 7.3
- Effort: M
- Risk: low (additive)
- Deps: —
- Files: .github/workflows/release.yml, scripts/install.sh

## Goal

Today the release workflow has `permissions: contents: write` and produces
unsigned tarballs. Add SLSA build provenance and (optionally) cosign
signatures so installers can verify provenance, not just checksum.

## Acceptance

- Workflow gains `permissions: id-token: write, attestations: write,
  contents: write`.
- A step runs `actions/attest-build-provenance@v1` for each tarball.
- Optionally: a step runs `cosign sign-blob` keyless against each tarball
  and uploads `*.sig` and `*.bundle` to the release.
- `install.sh` has a verification path: prefer `gh attestation verify` if
  `gh` is on PATH, fall back to `cosign verify-blob` if `cosign` is on
  PATH, fall back to checksum-only with a warning.
- README "Install" section explains all three verification levels.

## Notes

GitHub's attestation step is free and sufficient for "trust the build came
from this repo at this commit." Cosign adds the option to verify outside
the GitHub trust chain.
