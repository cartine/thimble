# Security Policy

## Reporting a Vulnerability

If you've found a vulnerability in Thimble, please report it privately so we
can fix it before public disclosure.

Two channels, in order of preference:

1. **GitHub Private Vulnerability Reporting** — open a draft advisory at
   <https://github.com/cartine/thimble/security/advisories/new>. This routes
   directly to maintainers, keeps the discussion private, and produces a CVE
   automatically when published.
2. **Email** — `security@cartine.me`. PGP fingerprint will be added here once
   the project has one published.

Please include:

- A description of the issue and its impact.
- Steps to reproduce, ideally with a minimal proof of concept.
- The version (`thimble --version`) you reproduced against, if applicable.
- Whether you intend to disclose publicly and on what timeline.

## Response SLA

We aim to:

- **Acknowledge** receipt within **3 business days**.
- **Triage** (severity assessment, CVE if warranted) within **7 business days**.
- **Patch** critical issues in `main` within **30 calendar days** of confirmation,
  shorter for actively exploited issues.
- **Credit** reporters publicly in release notes unless asked otherwise.

## Supported Versions

| Version    | Supported          |
|------------|--------------------|
| `main`     | ✓ (rolling)        |
| `0.x`      | ✓ (latest minor)   |
| `< 0.x-1`  | ✗                  |

We are pre-1.0. Until 1.0, only the latest `0.x` minor receives security fixes.
After 1.0, the policy will widen to the current major and one prior minor.

## Threat Model

A short threat model lives in the [README](README.md#threat-model). Internal
review notes from the initial implementation are at
[docs/security-review.md](docs/security-review.md).

### Residual risks

- **Compromise of the `age` binary on `PATH`.** Thimble shells out to `age`
  for every encrypt/decrypt. If pinning is not used, a malicious binary
  earlier in the path can intercept plaintext during encrypt and capture
  the identity-file path during decrypt, with zero indication. Mitigate
  per-invocation with `--age-binary=/path/to/age` (or
  `THIMBLE_AGE_BINARY=...`) and, when you have a known-good build, set
  `THIMBLE_AGE_SHA256=<hex>` so a mismatch aborts before the binary runs.
  K-18 is the gap; K-29 (`thimble doctor`) will surface the resolved path
  and SHA-256 on demand.

## Scope

In scope: the `thimble` CLI, the local web UI, the release tooling, and the
install scripts. Out of scope: vulnerabilities in `age`, the Go toolchain, or
other upstream dependencies — please report those to their respective projects.

## Public Disclosure

We coordinate disclosure. If a fix is available, we publish the advisory and
release notes simultaneously. If no fix is available within 90 days, we work
with the reporter on a mutually agreeable disclosure timeline.
