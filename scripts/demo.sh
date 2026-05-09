#!/usr/bin/env bash
# scripts/demo.sh — replayable Thimble lifecycle demo.
#
# Designed for asciinema. The recording covers:
#   age-keygen -> init -> recipient add --bootstrap -> set --origin=provision
#                -> list -> render
#
# Sample names match README "Real-Life Flow" so a viewer can connect demo→docs.
#
# Safety:
# - Everything happens inside a temp dir created by `mktemp -d`.
# - `provision | set --origin=provision` keeps generated values off the screen.
# - `render` is piped to /dev/null so no plaintext lands in the recording.
# - The temp dir is removed on EXIT (success or failure).
#
# Recording it:
#   asciinema rec assets/demo.cast bash scripts/demo.sh
#
# This is a viewer-paced narrative; the binary itself does no extra waiting.

set -euo pipefail

PAUSE="${PAUSE:-0.4}"

THIMBLE_BIN="${THIMBLE_BIN:-thimble}"
if ! command -v "$THIMBLE_BIN" >/dev/null 2>&1; then
  if [ -x ./thimble ]; then
    THIMBLE_BIN=./thimble
  else
    echo "demo: '$THIMBLE_BIN' not found on PATH and no ./thimble binary." >&2
    echo "      build it first: go build ./cmd/thimble" >&2
    exit 1
  fi
fi

if ! command -v age-keygen >/dev/null 2>&1 || ! command -v age >/dev/null 2>&1; then
  echo "demo: requires 'age' and 'age-keygen' on PATH." >&2
  exit 1
fi

DEMO_TMP="$(mktemp -d -t thimble-demo-XXXXXX)"
cleanup() {
  rm -rf "$DEMO_TMP"
}
trap cleanup EXIT INT TERM

export THIMBLE_STORE="$DEMO_TMP/secrets"
export THIMBLE_AGE_IDENTITY="$DEMO_TMP/identity.txt"

say() {
  printf '\n$ %s\n' "$*"
  sleep "$PAUSE"
}

# Generate a throwaway identity for the operator and a second one for a
# deploy-host recipient — added later via the bootstrap flow.
say "age-keygen -o \$THIMBLE_AGE_IDENTITY"
age-keygen -o "$THIMBLE_AGE_IDENTITY" 2>/dev/null
OPERATOR_RECIPIENT="$(age-keygen -y "$THIMBLE_AGE_IDENTITY")"

DEPLOY_IDENTITY="$DEMO_TMP/deploy.txt"
age-keygen -o "$DEPLOY_IDENTITY" 2>/dev/null
DEPLOY_RECIPIENT="$(age-keygen -y "$DEPLOY_IDENTITY")"

say "thimble init web-api production --recipient \$OPERATOR_RECIPIENT"
"$THIMBLE_BIN" init web-api production --recipient "$OPERATOR_RECIPIENT"
sleep "$PAUSE"

say "thimble recipient add --bootstrap web-api production \$DEPLOY_RECIPIENT"
"$THIMBLE_BIN" recipient add --bootstrap web-api production "$DEPLOY_RECIPIENT"
sleep "$PAUSE"

say "thimble provision | thimble set --origin=provision web-api production SESSION_SECRET"
"$THIMBLE_BIN" provision | "$THIMBLE_BIN" set --origin=provision \
  web-api production SESSION_SECRET
sleep "$PAUSE"

say "thimble provision | thimble set --origin=provision web-api production DATABASE_URL"
"$THIMBLE_BIN" provision | "$THIMBLE_BIN" set --origin=provision \
  web-api production DATABASE_URL
sleep "$PAUSE"

say "thimble list web-api production"
"$THIMBLE_BIN" list web-api production
sleep "$PAUSE"

say "thimble render --format dotenv web-api production > /dev/null   # values stay private"
"$THIMBLE_BIN" render --format dotenv web-api production > /dev/null
sleep "$PAUSE"

printf '\n# done — temp store at %s removed on exit.\n' "$DEMO_TMP"
