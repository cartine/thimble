# Using Thimble in CI/CD

Thimble collapses the "N secrets pasted into the CI platform" problem
into **one root secret**: the CI runner's age identity. Everything else
lives in the encrypted store, versioned and audited like any other
file.

## The pattern

1. Generate a dedicated age identity for CI (never reuse a human
   operator's identity) and grant its recipient to each namespace the
   pipeline needs:

   ```sh
   age-keygen -o ci-identity.txt          # do this locally, once
   age-keygen -y ci-identity.txt          # → age1ci...
   thimble recipient add web-api production age1ci...
   ```

   If the namespace has a quorum policy, the add goes through the
   normal challenge/sign-add flow — see
   [team-onboarding.md](team-onboarding.md).

2. Store the **contents of the identity file** as the CI platform's
   native secret (e.g. a GitHub Actions repository secret named
   `THIMBLE_AGE_IDENTITY_CONTENTS`). Then delete the local copy. This
   is the single root secret.

3. At job time: check out (or rsync) the encrypted store, write the
   identity to a `0600` file, and run your command under
   `thimble exec`. No plaintext ever touches the workspace.

A useful property while wiring this up: key **names** are plaintext
manifest metadata, so `thimble list web-api production` works without
any identity — handy for pipeline steps that only need to know what
keys exist. Values always require the identity.

## GitHub Actions

```yaml
name: deploy

on:
  push:
    branches: [main]

env:
  THIMBLE_STORE: ${{ github.workspace }}/secrets
  # Optional hardening: pin the age binary and its checksum.
  THIMBLE_AGE_BINARY: /usr/local/bin/age
  THIMBLE_AGE_SHA256: <sha256-of-your-pinned-age-binary>
  # Read-mostly CI should never broadcast to peers.
  THIMBLE_PEER_PUSH: "off"

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Check out repo (includes the encrypted store)
        uses: actions/checkout@v4
        # Alternatively, pull the store from your store host:
        # rsync -av store-host:/srv/abc-secrets/ "$THIMBLE_STORE/"

      - name: Install thimble and age
        run: |
          # install pinned age to $THIMBLE_AGE_BINARY, and thimble,
          # via your preferred pinned-release mechanism
          ./scripts/install-tools.sh

      - name: Write CI identity to a 0600 file
        run: |
          umask 077
          printf '%s\n' "$IDENTITY" > "$RUNNER_TEMP/thimble-identity.txt"
          chmod 0600 "$RUNNER_TEMP/thimble-identity.txt"
          echo "THIMBLE_AGE_IDENTITY=$RUNNER_TEMP/thimble-identity.txt" \
            >> "$GITHUB_ENV"
        env:
          IDENTITY: ${{ secrets.THIMBLE_AGE_IDENTITY_CONTENTS }}

      - name: Sanity check
        run: |
          thimble doctor
          thimble list web-api production   # key names only, never values
          thimble verify web-api production

      - name: Deploy
        run: |
          thimble exec web-api production -- ./scripts/deploy.sh

      - name: Shred identity
        if: always()
        run: rm -f "$RUNNER_TEMP/thimble-identity.txt"
```

Notes on the choices above:

- **`thimble exec ... -- <cmd>`** decrypts the namespace and pipes the
  values as dotenv to the child's **stdin** by default — nothing is
  written to the workspace and nothing appears in the job's
  environment listing. Prefer this over `--env`; reach for
  `render` only when a downstream tool genuinely needs a file, and
  then write it to a `0600` path you delete in an `always()` step.
- **`THIMBLE_PEER_PUSH: "off"`** (or `--no-peer-push` per mutation)
  keeps a read-mostly CI runner from attempting the on-mutate rsync
  broadcast to peers. CI usually has no ssh reachability to your
  leaders and should not be a writer anyway.
- **`THIMBLE_AGE_BINARY` + `THIMBLE_AGE_SHA256`** pin exactly which
  `age` binary Thimble shells out to and refuse to run if its SHA-256
  does not match — this closes off a PATH-hijack on shared runners.

## GitLab CI sketch

Store the identity contents as a **masked, protected** CI/CD variable
`THIMBLE_AGE_IDENTITY_CONTENTS` (File-type variables also work, but
set the mode yourself):

```yaml
deploy:
  stage: deploy
  variables:
    THIMBLE_STORE: "$CI_PROJECT_DIR/secrets"
    THIMBLE_PEER_PUSH: "off"
  script:
    - umask 077
    - printf '%s\n' "$THIMBLE_AGE_IDENTITY_CONTENTS" > /tmp/thimble-identity.txt
    - chmod 0600 /tmp/thimble-identity.txt
    - export THIMBLE_AGE_IDENTITY=/tmp/thimble-identity.txt
    - thimble verify web-api production
    - thimble exec web-api production -- ./scripts/deploy.sh
  after_script:
    - rm -f /tmp/thimble-identity.txt
```

## Cautions

- **Never echo decrypted output into logs.** `thimble render <app>
  <env> --format dotenv` prints plaintext to stdout by design — in CI
  that stdout is the job log, forever. If you must use `render`,
  redirect straight to a `0600` file and delete it; never pipe it
  through `tee`, `cat`, or a debugging step.
- **Masking is a backstop, not a defense.** Register the identity as
  a masked/secret variable on your platform so accidental echoes are
  redacted, but do not rely on it: masking matches exact strings and
  misses transformed or multi-line values. The real defense is that
  plaintext values never hit stdout at all.
- **Prefer exec's stdin mode.** The default `thimble exec` delivery
  (dotenv on the child's stdin) never lands in `/proc/<pid>/environ`,
  the job log, or a workspace file. `--env` exists for tools that
  cannot read stdin, but environment variables leak into crash dumps
  and child processes — use it deliberately, not by habit. The same
  logic applies to single values: `thimble and-get <app> <env> KEY --
  <command>` over ad-hoc shell plumbing.
- **Never commit the identity.** The encrypted store is safe to check
  in; the identity file is not, under any circumstances. Keep the CI
  identity distinct per pipeline so offboarding a pipeline is one
  `thimble recipient remove --rotate` away.
- **Verify on every fresh checkout.** `thimble verify <app> <env>`
  recomputes the bundle SHA-256 and prints the recipient list —
  cheap insurance that the store the runner just fetched is the store
  you published.
