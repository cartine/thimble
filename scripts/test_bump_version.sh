#!/usr/bin/env bash
# scripts/test_bump_version.sh — bump-algorithm unit tests.
#
# Sources scripts/bump-version.sh with BUMP_VERSION_LIB_ONLY=1 so only
# the compute_next_version function is loaded.

set -euo pipefail

BUMP_VERSION_LIB_ONLY=1
# shellcheck source=bump-version.sh
. "$(dirname "$0")/bump-version.sh"

fail=0
check() {
  desc="$1"; current="$2"; kind="$3"; want="$4"
  got="$(compute_next_version "$current" "$kind" 2>&1 || true)"
  if [ "$got" = "$want" ]; then
    printf 'ok  %s\n' "$desc"
  else
    printf 'FAIL %s: current=%s kind=%s got=%q want=%q\n' \
      "$desc" "$current" "$kind" "$got" "$want" >&2
    fail=$((fail + 1))
  fi
}

# Standard bumps from a baseline.
check "patch from 0.1.0"      "0.1.0" "patch" "v0.1.1"
check "minor from 0.1.0"      "0.1.0" "minor" "v0.2.0"
check "major from 0.1.0"      "0.1.0" "major" "v1.0.0"
check "patch resets nothing"  "0.1.5" "patch" "v0.1.6"
check "minor resets patch"    "0.1.5" "minor" "v0.2.0"
check "major resets minor+patch" "1.4.7" "major" "v2.0.0"

# Multi-digit components.
check "patch with two-digit"  "0.10.10" "patch" "v0.10.11"
check "minor across boundary" "0.9.9"   "minor" "v0.10.0"
check "major across boundary" "9.0.0"   "major" "v10.0.0"

# Explicit version.
check "explicit valid"        "0.1.0" "v2.3.4" "v2.3.4"
check "explicit also valid"   "9.9.9" "v0.0.1" "v0.0.1"

# No prior version — first-release defaults.
check "patch from no prior"   "" "patch" "v0.1.0"
check "minor from no prior"   "" "minor" "v0.1.0"
check "major from no prior"   "" "major" "v1.0.0"

if [ "$fail" -gt 0 ]; then
  printf '\n%d test(s) failed\n' "$fail" >&2
  exit 1
fi
printf '\nall bump tests passed\n'
