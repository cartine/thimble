<p align="center">
  <img src="assets/thimble-logo.svg" width="92" alt="Thimble logo">
</p>

# Thimble

Thimble is a small, file-first secrets manager for teams that want something
safer than shared `.env` files and lighter than a hosted Vault-shaped service.

It stores encrypted dotenv bundles in ordinary files, uses `age` for
recipient-based encryption, and keeps the working model intentionally narrow:
an application, an environment, a set of keys, and the public recipients allowed
to decrypt that bundle.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/cartine/thimble/main/scripts/install.sh | sh
```

Pin or update through the same installer (substitute a real tag from the
[releases page](https://github.com/cartine/thimble/releases) for `vX.Y.Z`):

```sh
THIMBLE_VERSION=vX.Y.Z curl -fsSL https://raw.githubusercontent.com/cartine/thimble/main/scripts/install.sh | sh
```

The install URL itself is pinned to a tag — not `main` — by [K-39](tasks/knots/K-39-install-pin-to-tag.md).

From a checkout:

```sh
go build ./cmd/thimble
```

Run `thimble doctor` after install to verify your setup.

## Requirements

- `age` on `PATH`.
- One age identity for each operator or deploy peer that needs to decrypt.
- `THIMBLE_AGE_IDENTITY=/path/to/identity.txt` or `--identity` when rendering,
  updating, or running the web UI against an existing namespace.

> **The `age` binary is a runtime trust boundary.** Thimble shells out to
> whichever `age` resolves on `PATH`. A malicious binary earlier in the path
> would silently capture plaintext on encrypt and the identity-file path on
> decrypt. Pin it explicitly to defend against this:
>
> - `--age-binary=/usr/local/bin/age` (or `THIMBLE_AGE_BINARY=...`) to skip
>   `PATH` lookup and use an absolute path.
> - `THIMBLE_AGE_SHA256=<hex>` to require the resolved binary to match a
>   known SHA-256 before each invocation.
> - `thimble --verbose` prints the resolved path once on first encrypt or
>   decrypt; useful for confirming what your shell actually picked up.

Generate an identity with:

```sh
age-keygen -o ~/.config/thimble/identity.txt
age-keygen -y ~/.config/thimble/identity.txt
```

The `age-keygen -y` output is the public recipient. The identity file is private
key material and should never be committed.

Thimble refuses to use an identity file whose mode allows group or world reads
(any bit in `0o077`). `age-keygen -o` creates files at `0600` by default, but
`cp`/`scp`/`mv` can drop permissions; if Thimble rejects yours, run
`chmod 0600 <path>` and retry. On filesystems that cannot represent that mode
(some Windows / WSL setups), pass `--unsafe-allow-identity-mode` to bypass the
check; a warning is logged to stderr each run.

## Namespaces

Thimble namespaces are `<application>/<environment>`.

```text
secrets/
  thimble.json
  web-api/
    production.env.age
    staging.env.age
```

The application name groups one deployable thing, such as `web-api`,
`worker`, or `admin-ui`. The environment name separates runtime contexts, such
as `production`, `staging`, or `local`.

Each namespace has its own encrypted bundle and its own recipient list. That
means `web-api/production` can include a deploy host recipient while
`web-api/staging` stays operator-only, even when both environments use the same
key names.

## Safe Secret Entry

Thimble does not accept secret values as command arguments. Arguments are too
easy to leak through shell history, process listings, terminal scrollback, and
agent transcripts.

Use the masked prompt:

```sh
thimble set web-api production DATABASE_URL
```

Or pipe from another command:

```sh
pass show web-api/production/database-url | thimble set web-api production DATABASE_URL
```

Generate a new random secret without displaying it:

```sh
thimble provision | thimble set web-api production SESSION_SECRET
```

For command chaining, use `and-set` so the generated value is captured and
stored without appearing on the terminal:

```sh
thimble and-set web-api production WEBHOOK_SECRET -- ./scripts/create-webhook-secret
```

`and-set` captures the producer's stderr by default; only the failure message
shows it (truncated and redacted). Pass `--show-stderr` if you need to see the
producer's stderr live for debugging — useful when the producer runs under
`set -x` and you trust your terminal not to be over the shoulder of someone
who shouldn't see it.

Use `and-get` to pass a secret to a command on stdin:

```sh
thimble and-get web-api production DATABASE_URL -- ./scripts/check-db
```

If a legacy tool can only read an environment variable, make that explicit:

```sh
thimble and-get --env DATABASE_URL web-api production DATABASE_URL -- ./scripts/deploy
```

Prefer stdin when possible. Environment variables are useful for compatibility,
but they are easier for child processes and debugging tools to expose.

Thimble refuses `and-get --env` when the child is one of `sh`, `bash`, `zsh`,
`fish`, `pwsh`, `cmd.exe`, or `powershell` — a shell will export the value to
its descendants and to anything its scripts run. The same guard applies to
`docker run` / `podman run` unless the invocation explicitly scopes the value
with `-e KEY`, `--env=KEY`, or `--env-file=-` (reading from stdin). Pass
`--allow-shell-env` if you genuinely want the wider exposure (e.g. an
interactive REPL where you want the child shell to see the secret); the guard
is opt-out for exactly that reason.

## Everyday CLI

```sh
thimble init web-api production --recipient age1operator...
thimble recipient add web-api production age1deployhost...

thimble create web-api production DATABASE_URL
thimble update web-api production DATABASE_URL
thimble delete web-api production OLD_TOKEN

thimble list web-api production
thimble render web-api production --format dotenv
thimble verify web-api production
thimble audit web-api production
thimble doctor
```

`list` shows keys only. `render` is the deliberate escape hatch for deployment
or local debugging, so treat its stdout as secret material. `verify` recomputes
the bundle's SHA-256 against the manifest and shows the recipient list. `audit`
prints the local append-only ledger of mutating ops for the namespace; entries
record an opaque operator thumbprint, never the recipient string or any
secret value.

## Real-Life Flow

Imagine a small service deployed by one operator and one deploy host.

1. The operator creates an age identity and records the public recipient.
2. The deploy host creates its own age identity and shares only its public
   recipient.
3. The operator initializes the production namespace:

   ```sh
   thimble init web-api production \
     --recipient age1operator... \
     --recipient age1deployhost...
   ```

4. The operator sets required secrets:

   ```sh
   thimble set web-api production DATABASE_URL
   thimble provision | thimble set web-api production SESSION_SECRET
   ```

5. The encrypted bundle is committed:

   ```sh
   git add secrets/thimble.json secrets/web-api/production.env.age
   git commit -m "add web-api production secrets"
   git push
   ```

6. The deploy host pulls the repository and renders only at deploy time:

   ```sh
   THIMBLE_AGE_IDENTITY=/etc/thimble/identity.txt \
     thimble render web-api production --format dotenv > /run/web-api.env
   chmod 0600 /run/web-api.env
   ```

The repository carries encrypted bundles and metadata. The operator laptop and
deploy host carry private identities. No peer needs another peer's private key.

## Peer-To-Peer Sync

Thimble's durable object is the encrypted bundle, not a central server. Git is
the simplest peer transport:

```text
operator laptop  ->  git remote  ->  deploy host
backup machine   ->  git remote  ->  operator laptop
```

Adding a peer securely:

1. The peer generates an age identity locally.
2. The peer shares only the public recipient.
3. An existing operator verifies that recipient out of band, such as in a call
   or an already trusted channel.
4. The operator runs:

   ```sh
   thimble recipient add web-api production age1peer...
   ```

5. The encrypted bundle and `thimble.json` are committed and synced.

Removing a peer:

```sh
thimble recipient remove web-api production age1peer...
git add secrets/thimble.json secrets/web-api/production.env.age
git commit -m "remove retired production recipient"
```

After removing a peer, rotate any high-risk values they could previously
decrypt. Recipient removal prevents future decrypts of newly encrypted bundles;
it cannot erase copies a former recipient already had.

Peer safety rules:

- Commit encrypted `.env.age` bundles, never plaintext `.env` files.
- Never commit age identity files.
- Keep at least one offline recovery recipient.
- Verify new recipients outside the git diff before granting access.
- Review recipient-only diffs as carefully as secret changes.
- Review `bundle_sha256` changes alongside ciphertext changes — a recipient-only
  diff that also bumps the SHA is the canonical re-encrypt; one without the
  other is a red flag.

Run `thimble verify <app> <env>` to recompute the on-disk bundle's SHA-256 and
print the match verdict alongside the recipient list. Use it on a fresh clone
to confirm nothing has been swapped under you.

## Web UI

```sh
thimble web
```

The UI binds to `127.0.0.1:8787` by default and prints a one-time token. The
first visit serves a paste-token form; on submit the server sets an HttpOnly,
SameSite=Strict session cookie. The UI can create namespaces, manage recipients,
and delete or list redacted keys. Binding to a non-loopback address requires
`--token` or `THIMBLE_WEB_TOKEN`.

A Host-header allowlist guards against DNS rebinding: requests are accepted
only when `Host` matches `127.0.0.1`, `[::1]`, `localhost`, or the configured
`--addr`. Use repeatable `--allow-host=foo.local:8787` to add other names for
non-loopback configurations.

The web token rotates after fifteen minutes of no authorized requests; tune
the window with `--idle-rotate=10m` (or pass `--idle-rotate=0` to disable
automatic rotation entirely). On Unix-like systems, `kill -USR1 <pid>` forces
an immediate rotation — useful when you suspect the token may have leaked
into a shared terminal or screen recording. Each rotation prints
`web token rotated; current token printed below:` followed by the new value
on stdout, and any cookies issued before the rotation stop authorizing
immediately, so existing browser tabs land back on the login form.

The web UI is **strict-mode only** for secret values: the browser never
accepts plaintext. Each key shown in the UI is paired with the exact CLI
command needed to set or update it (e.g. `thimble set api production
DB_URL`). Form bodies, browser autofill, refresh-resubmit, and DevTools
network panels are all out of scope for typing secrets — the CLI's masked
prompt or a pipe is the only path.

The browser UI is an operator convenience. Existing values are never displayed;
use `render` or `and-get` only when a deployment or command really needs the
plaintext.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, coding standards
(<500 lines/file, <100 lines/function, <100 columns/line), and the
PR workflow. Run `make lint` before committing.

[TAXONOMY.md](TAXONOMY.md) defines the project's shared vocabulary
(application, environment, namespace, recipient, identity, bundle, …).

## Threat Model

**In scope** — these are what Thimble is built to defend against:

| Threat | Mitigation |
|---|---|
| Lost laptop with an identity file | Recipients in encrypted bundles + `age` ChaCha20 — bundles in git remain unreadable to a finder. Rotate after loss. |
| Repo write-access attacker smuggling a recipient | Recipient validation + (post-K-36) quorum-signed recipient list. Today: review recipient diffs out of band. |
| Network MITM during install | `scripts/install.sh` verifies SHA-256 against the published checksums file (mandatory after K-38). |
| Sigstore-style provenance attacks on releases | `gh attestation verify` + cosign verification (post-K-40). |
| Accidental argv leak via shell history / `ps` | CLI rejects secret values as command arguments. Use the masked prompt, pipes, `provision`, or `and-set`. |
| Terminal scrollback / screen-share exposure | `provision` refuses TTY output without `--show`; `list`/web UI never display values. |
| Web UI surface (DNS rebinding, token leak) | Loopback-only by default; token-authenticated; cookie auth + Host-header allowlist (post-K-30 / K-31). |
| Concurrent operator edits silently dropping a key | Manifest version + flock (post-K-21). |

**Out of scope** — Thimble cannot defend against:

| Threat | Why not |
|---|---|
| Root on the deploy host | The application has to read the secret to run. Anything root can read, root can read. |
| Compromise of the `age` binary on `PATH` | Mitigated by pinning (K-18); but if the trust anchor itself is wrong, all bets are off. |
| Malicious operator with a valid identity | They can decrypt anything that identity can decrypt. Mitigation: rotate values and remove the recipient. |
| Side-channel attacks on the TTY (key timing, EM emanation, …) | We don't model these. |
| Cryptographic break of `age` | We don't model this. Rely on `age`'s threat model for the primitives. |

For the active hardening rollout closing each of these gaps, see
[tasks/knot-plan.md](tasks/knot-plan.md). Disclosure: see
[SECURITY.md](SECURITY.md).

## Security Position

- Encryption and decryption are delegated to `age`.
- Encrypted writes and metadata writes are atomic.
- Store directories are created `0700`; encrypted bundles and metadata are
  written `0600`.
- Secret values are rejected in argv.
- Interactive entry uses a masked terminal prompt.
- `provision`, `and-set`, and `and-get` support command flows that avoid
  printing values.
- CLI listing and the web UI do not display secret values.
- The web UI is loopback-only by default and token protected.

See [SECURITY.md](SECURITY.md) for the disclosure policy.

## License

Licensed under the [Apache License, Version 2.0](LICENSE).
