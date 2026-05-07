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

Pin or update through the same installer:

```sh
THIMBLE_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/cartine/thimble/main/scripts/install.sh | sh
```

From a checkout:

```sh
go build ./cmd/thimble
```

## Requirements

- `age` on `PATH`.
- One age identity for each operator or deploy peer that needs to decrypt.
- `THIMBLE_AGE_IDENTITY=/path/to/identity.txt` or `--identity` when rendering,
  updating, or running the web UI against an existing namespace.

Generate an identity with:

```sh
age-keygen -o ~/.config/thimble/identity.txt
age-keygen -y ~/.config/thimble/identity.txt
```

The `age-keygen -y` output is the public recipient. The identity file is private
key material and should never be committed.

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

## Everyday CLI

```sh
thimble init web-api production --recipient age1operator...
thimble recipient add web-api production age1deployhost...

thimble create web-api production DATABASE_URL
thimble update web-api production DATABASE_URL
thimble delete web-api production OLD_TOKEN

thimble list web-api production
thimble render web-api production --format dotenv
```

`list` shows keys only. `render` is the deliberate escape hatch for deployment
or local debugging, so treat its stdout as secret material.

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

## Web UI

```sh
thimble web
```

The UI binds to `127.0.0.1:8787` by default and prints a tokenized URL. It can
create namespaces, manage redacted keys, and add or remove recipients. Binding
to a non-loopback address requires `--token` or `THIMBLE_WEB_TOKEN`.

The browser UI is an operator convenience. Existing values are never displayed;
use `render` or `and-get` only when a deployment or command really needs the
plaintext.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, coding standards
(<500 lines/file, <100 lines/function, <100 columns/line), and the
PR workflow. Run `make lint` before committing.

[TAXONOMY.md](TAXONOMY.md) defines the project's shared vocabulary
(application, environment, namespace, recipient, identity, bundle, …).

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
