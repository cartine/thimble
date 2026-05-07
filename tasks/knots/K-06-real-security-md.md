# K-06 — Replace SECURITY.md with vuln-reporting policy

- Wave / Step: 1.2
- Effort: S
- Risk: low
- Deps: —
- Files: SECURITY.md (rewrite), docs/security-review.md (new)

## Goal

The current `SECURITY.md` is internal review notes, not a disclosure policy.
GitHub auto-detects `SECURITY.md` and surfaces it in the Security tab — it
should answer "how do I report a vulnerability?" and "which versions are
supported?" Move the existing review checklist into `docs/security-review.md`.

## Acceptance

- New `SECURITY.md` contains: (a) supported-versions table, (b) private
  reporting channel (e.g. GitHub Private Vulnerability Reporting enabled,
  plus an email contact), (c) expected response SLA.
- The "Security Agent Verdict: Approved" line is removed — it overclaims given
  the Residual Risks list.
- Existing review/checklist content moves to `docs/security-review.md` with a
  date stamp and reviewer attribution.
- GitHub Security tab now displays the new policy.

## Notes

GitHub PVR (Private Vulnerability Reporting) is free and the lowest-friction
inbox; enabling it from repo Settings → Security takes 30 seconds.
