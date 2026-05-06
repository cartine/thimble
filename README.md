# Thimble

Thimble is a small, file-first secrets manager for app/environment scoped dotenv
bundles. It keeps encrypted files in a local `secrets/` directory and shells out
to the audited `age` CLI for encryption instead of inventing a crypto format.

## Install

From GitHub releases:

```sh
curl -fsSL https://raw.githubusercontent.com/cartine/thimble/main/scripts/install.sh | sh
```

Update the same way, or pin a version:

```sh
THIMBLE_VERSION=v0.1.0 curl -fsSL https://raw.githubusercontent.com/cartine/thimble/main/scripts/install.sh | sh
```

For a checkout build:

```sh
go build ./cmd/thimble
```

## Requirements

- `age` must be on `PATH`.
- Set `THIMBLE_AGE_IDENTITY=/path/to/identity.txt` or pass `--identity` when
  decrypting existing namespaces.

## CLI

```sh
thimble init koja production --recipient age1operator...
thimble create koja production POSTGRES_PASSWORD
thimble update koja production POSTGRES_PASSWORD
thimble delete koja production OLD_KEY
thimble list koja production
thimble render koja production --format dotenv
```

Values passed as arguments are convenient but can land in shell history. Omit the
value to read it from stdin:

```sh
printf '%s' "$POSTGRES_PASSWORD" | thimble create koja production POSTGRES_PASSWORD
```

## Web UI

```sh
thimble web
```

The UI binds to `127.0.0.1:8787` by default and prints a one-time tokenized URL.
It lists keys only, redacts values, and lets an operator create namespaces,
create/update/delete secrets, and add/remove recipients. Binding to a non-local
address requires `--token` or `THIMBLE_WEB_TOKEN`.

## Storage

```text
secrets/
  thimble.json
  koja/
    production.env.age
```

`thimble.json` contains metadata only: applications, environments, encrypted
file names, recipients, and timestamps. Secret values live only in the age
encrypted bundle and in process memory during the command that needs them.

## Security Position

- Encryption and decryption are delegated to `age`.
- Encrypted writes and metadata writes are atomic.
- Store directories are created `0700`; encrypted bundles and metadata are
  written `0600`.
- CLI listing and the web UI do not display secret values.
- The web UI is loopback-only by default and token protected.
- Deploy hosts and operators should be treated as privileged recipients.

See [SECURITY.md](SECURITY.md) for the review checklist and residual risks.
