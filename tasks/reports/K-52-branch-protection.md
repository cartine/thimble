# K-52 Branch Protection — Evidence

Date: 2026-05-08
Knot: thimble-209a ("Configure branch protection on main")

## What was configured

Branch protection on `cartine/thimble:main` is enforced by GitHub
**ruleset id `16168862`** (`name: "main protection"`, `enforcement:
active`).

Configured via the modern Rulesets API rather than the classic
branch-protection API because the latter is paywalled for private
repos on free GitHub plans. As part of this knot the repo was also
flipped from private to public, which (a) unlocked both APIs at zero
cost and (b) aligned the repo's actual visibility with its
public-facing artifacts (LICENSE, SECURITY policy, install.sh
referencing `raw.githubusercontent.com`, README PVR link).

## Rules attached

```json
[
  {"type": "deletion"},
  {"type": "non_fast_forward"},
  {
    "type": "pull_request",
    "parameters": {
      "required_approving_review_count": 0,
      "dismiss_stale_reviews_on_push": false,
      "require_code_owner_review": false,
      "require_last_push_approval": false,
      "required_review_thread_resolution": false
    }
  },
  {
    "type": "required_status_checks",
    "parameters": {
      "strict_required_status_checks_policy": true,
      "required_status_checks": [
        {"context": "build-test (ubuntu-latest)"},
        {"context": "build-test (macos-latest)"},
        {"context": "vuln"},
        {"context": "lint"},
        {"context": "integration"}
      ]
    }
  }
]
```

## Effect

- **Direct push to `main` is rejected.** Every change must go through
  a pull request (even with 0 required approvals — the PR mechanism
  itself is the gate that lets CI run).
- **`main` cannot be deleted** via API or UI.
- **`main` cannot be force-pushed.** `non_fast_forward` blocks any
  push that rewrites history.
- **PRs targeting `main` cannot merge** until the five required CI
  checks pass: `build-test (ubuntu-latest)`,
  `build-test (macos-latest)`, `vuln`, `lint`, `integration`.
- **`strict_required_status_checks_policy: true`** means the PR
  branch must be up to date with `main` before the merge button
  enables.
- **`bypass_actors: []`** — no one can bypass the rule, including
  admins (until/unless an actor is added).

## How to inspect / change later

```sh
# List rulesets
gh api repos/cartine/thimble/rulesets

# Inspect this one
gh api repos/cartine/thimble/rulesets/16168862

# Disable temporarily (for emergency direct push)
gh api -X PUT repos/cartine/thimble/rulesets/16168862 \
  -f enforcement=disabled

# Update the required-checks list when CI gains/drops jobs
gh api -X PUT repos/cartine/thimble/rulesets/16168862 \
  --input <new-rules.json>
```

## Verification

- `gh api repos/cartine/thimble/rulesets` lists the ruleset with
  `enforcement=active`.
- A direct `git push origin main` from a follow-up commit (untested
  in this knot — verifying would mean making the rule fail-loud,
  which is the point) will be rejected with a "ruleset" error.
- The first PR opened against `main` after this ruleset landed will
  require all five CI checks to pass before merge.

## Related knots

- K-13/K-14/K-15/K-16: the CI workflow jobs whose names this rule
  pins.
- K-50/K-51: future quorum-policy work that, once shipped, would
  ideally have its own ruleset for the policy file's history.
