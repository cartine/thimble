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

- **Single-operator recipient additions when no quorum policy is
  configured.** When `secrets/recipients.signed.toml` is absent, any
  operator with merge access to the consumer repo can grant a new
  recipient via a one-line manifest diff. K-36 ships the structural
  fix as **opt-in**: drop a policy file listing M-of-N operators and
  every recipient add must be signed by M of them before the bundle
  re-encrypts. Without the policy file, the only mitigations are
  out-of-band review of recipient diffs and the K-27 audit log.
  Protocol detail: [docs/recipient-quorum.md](docs/recipient-quorum.md).

- **Removing a recipient does not invalidate plaintext or encrypted
  copies they already obtained.** Once a peer has decrypted a bundle
  the plaintext can outlive the recipient list. K-37 makes rotation
  the easy default: `thimble recipient remove --rotate <app> <env>
  age1...` regenerates every value whose origin is `provision`
  (high-entropy random tokens produced by `thimble provision`)
  atomically alongside the recipient drop, and surfaces every other
  key as "manual rotate needed" so the operator knows what to re-set
  out of band. The new value lands under the same exclusive flock
  as the recipient list, so a concurrent reader either sees the old
  bundle or the fully-rotated bundle, never a torn state. The
  removed peer's already-decrypted plaintext remains a residual risk
  — anything the peer copied before removal is out of Thimble's
  control — but the post-rotation bundle is unreadable to them and
  the new values are unknown to them.

## Scope

In scope: the `thimble` CLI, the local web UI, the release tooling, and the
install scripts. Out of scope: vulnerabilities in `age`, the Go toolchain, or
other upstream dependencies — please report those to their respective projects.

### Web UI scope

The web UI is a **redacted viewer plus recipient/key manager**. It can:

- Create namespaces.
- List keys (never values) per namespace.
- Add and remove recipients (these are public addresses, not secrets).
- Delete keys.

It cannot accept secret values. Strict-mode rejection (K-34) returns a 400
on any POST that carries a non-empty `value` form field; the rejection body
points the operator at the CLI command. Setting or updating a key is always
done from the terminal via `thimble set <app> <env> <KEY>` so plaintext
never touches form bodies, browser autofill, or DevTools.

## Public Disclosure

We coordinate disclosure. If a fix is available, we publish the advisory and
release notes simultaneously. If no fix is available within 90 days, we work
with the reporter on a mutually agreeable disclosure timeline.
