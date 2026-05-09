# Thimble Knot Execution Plan

A "knot" is one self-contained unit of work with a clear acceptance condition.
Knots are grouped into waves; within a wave, steps are sequential where the
"deps" line says so, otherwise parallel-safe. Each knot lives in
`tasks/knots/K-NN-slug.md`.

## Conventions

- ID: `K-NN`, monotonic.
- Wave / Step: `W.S`, e.g. `0.1`, `4.3`.
- Risk: `low | med | high` — operator-felt blast radius if shipped wrong.
- Effort: rough t-shirt — `S` (≤2h), `M` (≤1d), `L` (≤3d), `XL` (>3d).
- Deps: list of upstream knot IDs that must land first.
- Acceptance: a binary condition a reviewer can check without running code.

## Wave Map

| Wave | Theme                              | Parallel-safe? | Gate to next wave                      |
|------|------------------------------------|----------------|----------------------------------------|
| 0    | Source-shape & vocabulary baseline | No (sequential) | Code conforms to size budgets and TAXONOMY.md |
| 1    | Legal & trust scaffolding          | Yes            | Repo passes basic procurement smell-test |
| 2    | Code structure                     | No             | `main.go` split; package boundaries land |
| 3    | CI / quality gates                 | Yes            | PRs run vet, test, lint, vuln, real-`age` integration |
| 4    | Runtime security hardening         | Yes            | Threat-model items 1–11, 22–23 closed   |
| 5    | Web UI hardening                   | Yes            | Threat-model items 12–15 closed         |
| 6    | Recipient governance               | No             | Recipient adds gated; rotate flow exists |
| 7    | Release & install hardening        | Yes            | Signed, attested, multi-channel installs |
| 8    | DevX polish                        | Yes            | README/Make/automation tell the story   |

## Wave 0 — Foundation Alignment (sequential)

Wave 0 must run end-to-end, in order, before any other wave starts. It
establishes the size budgets and naming taxonomy that subsequent waves are
expected to honor. Splitting `main.go` (Wave 2) is purely mechanical once these
are settled.

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 0.1  | K-01   | Run `/enforce-sourcecode-size`                     |
| 0.2  | K-02   | Run `/taxonomize`                                  |
| 0.3  | K-03   | Run `/taxonomize-align-docs`                       |
| 0.4  | K-04   | Run `/taxonomize-align-code`                       |

## Wave 1 — Legal & Trust Scaffolding (parallel)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 1.1  | K-05   | Add LICENSE (Apache-2.0 or MIT)                    |
| 1.2  | K-06   | Replace SECURITY.md with vuln-reporting policy     |
| 1.3  | K-07   | Add CONTRIBUTING.md                                |
| 1.4  | K-08   | Add CHANGELOG.md (Keep-a-Changelog)                |
| 1.5  | K-09   | Add Threat Model section to README                 |
| 1.6  | K-10   | Add status badges to README                        |
| 1.7  | K-11   | Pin README install example to a real tag           |

## Wave 2 — Code Structure (sequential, after Wave 0)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 2.1  | K-12   | Split main.go into packages                        |

## Wave 3 — CI / Quality Gates (parallel, after Wave 2)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 3.1  | K-13   | PR CI: go vet + go test                            |
| 3.2  | K-14   | PR CI: govulncheck                                 |
| 3.3  | K-15   | PR CI: staticcheck + gosec                         |
| 3.4  | K-16   | Integration test job against real `age` binary     |
| 3.5  | K-17   | Dependabot / Renovate config                       |

## Wave 4 — Runtime Security Hardening (parallel, after Wave 3)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 4.1  | K-18   | Pin/verify `age` binary path                       |
| 4.2  | K-19   | Identity-file mode sanity check                    |
| 4.3  | K-20   | Strict recipient format validation                 |
| 4.4  | K-21   | Manifest version + flock (TOCTOU fix)              |
| 4.5  | K-22   | Bundle attestation (ciphertext SHA + signed manifest) |
| 4.6  | K-23   | Suppress producer stderr by default                |
| 4.7  | K-24   | `and-get --env` shell/docker guard                 |
| 4.8  | K-25   | Scanner buffer guard for huge values               |
| 4.9  | K-26   | Timeout + cancellable context for `age` subprocess |
| 4.10 | K-27   | Append-only audit log                              |
| 4.11 | K-28   | `thimble --version` subcommand                     |
| 4.12 | K-29   | `thimble doctor` subcommand                        |

## Wave 5 — Web UI Hardening (parallel, after Wave 2)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 5.1  | K-30   | Move web token from URL to cookie                  |
| 5.2  | K-31   | Host-header allowlist (DNS rebinding defense)      |
| 5.3  | K-32   | `Cache-Control: no-store` on web responses         |
| 5.4  | K-33   | Web token rotation (`--rotate-token`)              |
| 5.5  | K-34   | Plaintext-input policy on web UI                   |
| 5.6  | K-35   | Startup banner: scope warning                      |

## Wave 6 — Recipient Governance (sequential)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 6.1  | K-36   | Quorum-signed recipient list                       |
| 6.2  | K-37   | `recipient remove --rotate` flow                   |

## Wave 7 — Release & Install Hardening (parallel, after Wave 3)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 7.1  | K-38   | install.sh: mandatory checksum verification        |
| 7.2  | K-39   | install.sh: pin to tag, not main                   |
| 7.3  | K-40   | Release: sigstore attestation + GH attest          |
| 7.4  | K-41   | Reproducible-build verify target                   |
| 7.5  | K-42   | Distribution: Homebrew tap                         |
| 7.6  | K-43   | Distribution: Scoop bucket                         |
| 7.7  | K-44   | Distribution: Debian (.deb) repo                   |
| 7.8  | K-45   | Distribution: Dockerfile + GHCR image              |

## Wave 8 — DevX Polish (parallel, last)

| Step | Knot   | Title                                              |
|------|--------|----------------------------------------------------|
| 8.1  | K-46   | Demo GIF / asciinema in README                     |
| 8.2  | K-47   | Makefile targets                                   |
| 8.3  | K-48   | `make tag-release` automation                      |

## Critical Path

```
W0 ── W1 ┐
   │     ├─→ W2 ── W3 ── W4
   │     │              └─ W5
   │     │              └─ W7
   │     └─→ W6 (depends on W4.K-20)
   │
   └─────→ W8
```

W0 gates everything. W1 is independent of W0 and can begin immediately, but
nothing in W2+ should land until W0 has stabilized the size budgets and
vocabulary the rest of the work is graded against.
