#!/usr/bin/env bash
# scripts/test_tag_release_bump.sh — bump-algorithm unit tests.
#
# Sources scripts/tag-release.sh with TAG_RELEASE_LIB_ONLY=1 so only
# the compute_next_version function is loaded. Exercises the bump
# matrix.
#
# Run via:
#   bash scripts/test_tag_release_bump.sh

set -euo pipefail

TAG_RELEASE_LIB_ONLY=1
# shellcheck source=tag-release.sh
. "$(dirname "$0")/tag-release.sh"

fail=0
check() {
  desc="$1"; latest="$2"; kind="$3"; want="$4"
  got="$(compute_next_version "$latest" "$kind" 2>&1 || true)"
  if [ "$got" = "$want" ]; then
    printf 'ok  %s\n' "$desc"
  else
    printf 'FAIL %s: latest=%s kind=%s got=%q want=%q\n' \
      "$desc" "$latest" "$kind" "$got" "$want" >&2
    fail=$((fail + 1))
  fi
}

# Standard bumps from a baseline.
check "patch from v0.1.0"      "v0.1.0" "patch" "v0.1.1"
check "minor from v0.1.0"      "v0.1.0" "minor" "v0.2.0"
check "major from v0.1.0"      "v0.1.0" "major" "v1.0.0"
check "patch resets nothing"   "v0.1.5" "patch" "v0.1.6"
check "minor resets patch"     "v0.1.5" "minor" "v0.2.0"
check "major resets minor+patch" "v1.4.7" "major" "v2.0.0"

# Multi-digit components.
check "patch with two-digit"   "v0.10.10" "patch" "v0.10.11"
check "minor across boundary"  "v0.9.9"   "minor" "v0.10.0"
check "major across boundary"  "v9.0.0"   "major" "v10.0.0"

# Explicit version.
check "explicit valid"         "v0.1.0" "v2.3.4" "v2.3.4"
check "explicit also valid"    "v9.9.9" "v0.0.1" "v0.0.1"

# No prior tag — first-release defaults.
check "patch from no prior tag" "" "patch" "v0.1.0"
check "minor from no prior tag" "" "minor" "v0.1.0"
check "major from no prior tag" "" "major" "v1.0.0"

if [ "$fail" -gt 0 ]; then
  printf '\n%d test(s) failed\n' "$fail" >&2
  exit 1
fi
printf '\nall bump tests passed\n'
