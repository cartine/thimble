# K-17 — Dependabot / Renovate config

- Wave / Step: 3.5
- Effort: S
- Risk: low
- Deps: —
- Files: .github/dependabot.yml (new)

## Goal

Drop-in upstream-update bot. Visible "I keep this current" signal and a real
defense against stale-CVE drift.

## Acceptance

- `.github/dependabot.yml` covers `gomod` (weekly) and `github-actions`
  (weekly).
- Auto-merge is *not* enabled for `gomod`. (We want the gate, not the auto.)
- A label policy exists so security updates land with `security` label.

## Notes

The current dep set is tiny (`golang.org/x/term`, `golang.org/x/sys`), so
churn will be low. The valuable part is the GitHub Actions update stream —
`actions/checkout`, `setup-go`, `softprops/action-gh-release` all need
periodic bumps.
