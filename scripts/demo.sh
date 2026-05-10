#!/usr/bin/env bash
# scripts/demo.sh — replayable Thimble lifecycle demo.
#
# Designed for asciinema. The recording covers the canonical secure-by-
# default flow:
#   age-keygen → init → recipient add --bootstrap → set --origin=provision
#   → list → verify → exec (the punch line: no FS writes, secret on stdin)
#   → audit
#
# Safety:
# - Everything happens inside a temp dir created by `mktemp -d`.
# - `provision | set --origin=provision` keeps generated values off the screen.
# - `thimble exec` pipes the dotenv body to a child that reads stdin and
#   prints "DATABASE_URL is set, length 43" — never the value itself.
# - The temp dir is removed on EXIT (success or failure).
#
# Recording it:
#   asciinema rec --overwrite -c "bash scripts/demo.sh" assets/demo.cast
#   make demo-gif    # converts to assets/demo.gif via `agg`
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

say "thimble verify web-api production"
"$THIMBLE_BIN" verify web-api production
sleep "$PAUSE"

# The punch line: thimble exec hands the entire decrypted namespace to a
# child via stdin. The child receives a dotenv body and prints only key
# names + value lengths — values themselves never reach the screen.
RECEIVER="$DEMO_TMP/receiver.sh"
cat > "$RECEIVER" <<'RECV'
#!/usr/bin/env bash
echo "# child app: reading stdin, never echoing values"
while IFS='=' read -r k v; do
  [ -z "$k" ] && continue
  printf '  %-20s set, length %d\n' "$k" "${#v}"
done
RECV
chmod 0755 "$RECEIVER"

say "thimble exec web-api production -- $RECEIVER   # secret stays in process memory"
"$THIMBLE_BIN" exec web-api production -- "$RECEIVER"
sleep "$PAUSE"

say "thimble audit --limit 5 web-api production   # one row per mutation"
"$THIMBLE_BIN" audit --limit 5 web-api production
sleep "$PAUSE"

printf '\n# done — temp store at %s removed on exit.\n' "$DEMO_TMP"
