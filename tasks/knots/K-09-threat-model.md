# K-09 — Add Threat Model section to README

- Wave / Step: 1.5
- Effort: S
- Risk: low
- Deps: —
- Files: README.md

## Goal

Honest threat models build trust faster than feature lists. Make the
in-scope/out-of-scope split explicit so users can decide if Thimble fits
their risk model in 30 seconds.

## Acceptance

- README gains a "Threat Model" section (one screen, not a paper) covering:
  - **In scope**: lost laptop with identity file, repo write-access attacker
    smuggling a recipient, network MITM during install, terminal
    history/scrollback exposure, accidental argv leak.
  - **Out of scope**: root on the deploy host (see thimble.md), `age`
    protocol break, malicious operator with a valid identity, side-channel
    attacks on the TTY.
- Each in-scope item points at the control that mitigates it (recipient
  validation, atomic writes, no-argv-secrets, masked prompt, etc.).
- Each out-of-scope item names the user's responsibility (e.g. "rotate after
  laptop loss; we cannot retroactively revoke past plaintext").

## Notes

This section also unblocks K-06 (real SECURITY.md can link here) and K-07
(CONTRIBUTING.md can link here).
