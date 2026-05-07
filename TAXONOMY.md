# Thimble Taxonomy

> Shared vocabulary for this codebase. Humans: edit freely — your definitions win.
> Agents: read before writing code. Update via `/taxonomize` (preserves human edits).

Last auto-run: 2026-05-07 · Scope: `.`

## How to read this file

- **Nouns** are domain entities and concepts.
- **Verbs** are operations performed on nouns.
- **Phrases** are compound terms that carry more meaning than their parts.
- Citations like `path/file.ext:42` anchor each term to real usage.
- `<!-- human -->` marks hand-written entries; `<!-- auto -->` marks generated ones.
- ⚠ markers flag things to review: `overloaded`, `ambiguous`, `stale`, `divergence`.

---

## Nouns

### age <!-- auto -->
The audited file-encryption tool Thimble shells out to. Not a noun in
Thimble's domain proper; treated as the trusted primitive boundary.
- `cmd/thimble/main.go:407` — `agePath: "age"`
- `cmd/thimble/main.go:679-707` — encrypt/decrypt invocations
- `README.md:34` — listed under Requirements
- See also: **identity**, **recipient**.

### and-get <!-- auto -->
CLI subcommand that decrypts one secret value and pipes it to a child
command's stdin (default) or environment variable (with `--env`).
- `cmd/thimble/main.go:114` — dispatch
- `cmd/thimble/main.go:303-328` — `runAndGet`
- `README.md:104-115`

### and-set <!-- auto -->
CLI subcommand that captures a child command's stdout and stores it as
a secret value, never echoing to the terminal.
- `cmd/thimble/main.go:113` — dispatch
- `cmd/thimble/main.go:283-301` — `runAndSet`
- `README.md:98-100`

### application <!-- auto -->
A deployable thing (e.g. `web-api`, `worker`, `admin-ui`) that owns one
or more environments. First positional arg in most CLI subcommands.
Aliases: `app` (used everywhere in code/CLI as the short form).
- `cmd/thimble/main.go:49-51` — `appManifest`
- `README.md:62-64`
- thimble.md uses both forms.

### audit log <!-- auto -->
⚠ stale — referenced in `SECURITY.md` Residual Risks and planned by
[K-27](tasks/knots/K-27-audit-log.md), not yet implemented in code.
- `SECURITY.md:42-43`

### bundle <!-- auto -->
The encrypted dotenv file written to disk. One per namespace. The
durable, peer-shippable artifact. Aliases: `encrypted bundle`,
`encrypted dotenv bundle`.
- `cmd/thimble/main.go:53-59` — `envManifest.File`
- `README.md:56-69`
- `thimble.md:50-58`

### CLI <!-- auto -->
The `thimble` command-line tool. Entry point in `cmd/thimble/main.go`.

### deploy host <!-- auto -->
A machine that decrypts a bundle at runtime to run the application.
Holds its own age identity. Aliases: `deploy peer`, `peer` (when context
is deployment).
- `README.md:135-170`
- `thimble.md:135-148`
- ⚠ overloaded with **peer** in some prose.

### dotenv <!-- auto -->
The plaintext key-value format Thimble reads and writes. Only supported
output of `render`.
- `cmd/thimble/main.go:735-808` — `parseDotenv` / `encodeDotenv`
- `README.md:128`

### environment <!-- auto -->
Runtime context within an application: `production`, `staging`, `local`.
Second positional arg in most CLI subcommands. Aliases: `env`.
- `cmd/thimble/main.go:53-59` — `envManifest`
- `README.md:62-69`
- ⚠ overloaded: `env` is also a CLI flag short for `--env` in `and-get`,
  and an OS environment variable. Use the full word `environment` in
  docs when ambiguity is possible.

### identity <!-- auto -->
A private age key file. Personal, never committed, used to decrypt
bundles. Operators and deploy hosts each have one. Configured via
`--identity` flag or `THIMBLE_AGE_IDENTITY` env var.
- `cmd/thimble/main.go:39-42` — `cliConfig.identity`
- `README.md:34-49`
- See also: **recipient** (the public counterpart).

### key <!-- auto -->
A secret name in dotenv form (uppercase, e.g. `DATABASE_URL`,
`SESSION_SECRET`). Validated by `keyPattern`.
- `cmd/thimble/main.go:36` — `keyPattern = ^[A-Z_][A-Z0-9_]*$`
- `cmd/thimble/main.go:930-935` — `validateKey`
- ⚠ overloaded: also informally used for "private key" in prose
  (identity files). Prefer **identity** for the latter.

### manifest <!-- auto -->
The `secrets/thimble.json` file. Maps applications → environments →
metadata (recipients, file path, timestamps). Plaintext by design.
- `cmd/thimble/main.go:33` — `manifestName = "thimble.json"`
- `cmd/thimble/main.go:44-59` — `manifest` / `appManifest` / `envManifest`
- `README.md:55-60`

### namespace <!-- auto -->
The pair `<application>/<environment>`. The unit of recipient-list
isolation. One bundle per namespace.
- `README.md:50-69`
- `cmd/thimble/main.go:1020-1025` — `namespaceView`
- thimble.md uses "environment" alone where this code uses
  "namespace"; see drift report.

### operator <!-- auto -->
A human running Thimble. Holds an age identity, manages bundles and
recipient lists. Aliases: `user`, `operator laptop` (when stressing the
machine).
- `README.md:134-173`
- `thimble.md:96-103`

### recipient <!-- auto -->
A public age (or ssh) key authorized to decrypt a bundle. Stored in the
manifest per namespace. Aliases: `public recipient`,
`age public recipient`.
- `cmd/thimble/main.go:53-59` — `envManifest.Recipients`
- `cmd/thimble/main.go:949-957` — `validateRecipient`
- `README.md:36-40`

### recovery recipient <!-- auto -->
An offline-stored age recipient kept as a break-glass for the case
where every active operator's identity is lost.
- `README.md:215`

### secret <!-- auto -->
A `(key, value)` pair living inside an encrypted bundle. The value is
plaintext only briefly (in memory during one command).
- `README.md:71-115` (Safe Secret Entry)
- `SECURITY.md:11-12`

### store <!-- auto -->
Two senses, now disambiguated by package qualifier (K-04 / K-12):
- (1) The on-disk secrets directory (default `secrets/`). Configured by
  `--store` or `THIMBLE_STORE`. `internal/cli/cli.go:18`,
  `internal/cli/cli.go:46`.
- (2) The Go type wrapping disk operations: `store.Store` in
  `internal/store/store.go`. The package qualifier `store.` is what
  resolves the historic ambiguity — bare `store` in code now always
  refers to the package, not the directory or the verb.
Prefer **store directory** for (1) when context is ambiguous.

### token <!-- auto -->
The web UI authentication secret. Generated automatically on loopback
binds; required explicitly for non-loopback. Compared with
`subtle.ConstantTimeCompare`.
- `cmd/thimble/main.go:381-403` — token plumbing
- `cmd/thimble/main.go:1153-1163` — `authorized`
- ⚠ overloaded with "session token" in informal usage; the web UI does
  not have separate sessions today (cookie support lands in K-30).

### web UI <!-- auto -->
The local HTTP server bound by default to `127.0.0.1:8787`. Operator
convenience for namespace/recipient/key management. Never displays
existing values.
- `cmd/thimble/main.go:377-404` — `runWeb`
- `cmd/thimble/main.go:1027-1394` — server, routes, template
- `README.md:219-232`

---

## Verbs

### add (recipient) <!-- auto -->
Append a recipient to a namespace and re-encrypt the bundle.
- `cmd/thimble/main.go:445-453` — `(*store).AddRecipient`
- Subject: **namespace**. Related: **remove**.

### and-get <!-- auto -->
See **and-get** noun entry; same word is the verb form.

### and-set <!-- auto -->
See **and-set** noun entry; same word is the verb form.

### claim <!-- auto -->
(kno) Claim a knot to begin work in its current state. Not a Thimble
verb but used throughout the rollout via `tasks/knot-plan.md`.
- See `.claude/skills/knots/SKILL.md`.

### create <!-- auto -->
CLI verb that adds a key to a namespace, failing if the key already
exists.
- `cmd/thimble/main.go:474-485` — `(*store).CreateSecret`
- Subject: **secret** in a **namespace**. Related: **update**, **set**.

### decrypt <!-- auto -->
Read and decrypt a bundle to plaintext (delegated to `age -d`).
- `cmd/thimble/main.go:694-709` — `(*store).decrypt`

### delete / rm <!-- auto -->
Remove a key from a namespace and re-encrypt.
- `cmd/thimble/main.go:510-521` — `(*store).DeleteSecret`
- Subject: **secret**.

### encrypt <!-- auto -->
Re-encrypt a namespace's plaintext to its current recipient list,
producing a new bundle (atomic write).
- `cmd/thimble/main.go:674-692` — `(*store).encryptAndWrite`

### init <!-- auto -->
CLI verb that creates a fresh namespace with an initial recipient list.
- `cmd/thimble/main.go:156-188` — `runInit`
- `cmd/thimble/main.go:410-443` — `(*store).Init`
- Subject: **namespace**.

### list / ls <!-- auto -->
Print the keys of a namespace. Never prints values.
- `cmd/thimble/main.go:342-354` — `runList`
- `cmd/thimble/main.go:523-534` — `(*store).ListSecrets`

### provision <!-- auto -->
Generate a random base64url secret of N≥16 bytes. Refuses to print to
a TTY without `--show`. Designed for piping into **set**.
- `cmd/thimble/main.go:258-281` — `runProvision`
- `README.md:91-93`

### redact <!-- auto -->
Trim and truncate stderr from `age` (or producer) before surfacing it
in error messages, to avoid accidental value leakage.
- `cmd/thimble/main.go:973-982`

### remove (recipient) <!-- auto -->
Drop a recipient from a namespace and re-encrypt. Refuses removal of
the last recipient.
- `cmd/thimble/main.go:455-472` — `(*store).RemoveRecipient`
- ⚠ See [K-37](tasks/knots/K-37-recipient-remove-rotate.md) — does not
  yet rotate values that the removed peer can still decrypt from older
  copies.

### render <!-- auto -->
Decrypt a namespace and emit dotenv to stdout. The deliberate
plaintext escape hatch.
- `internal/cli/run_secret.go` — `runRender`
- `internal/store/store.go` — `(*Store).Render`
- The historic three-way collision (CLI verb `render`,
  `(*store).Render`, `webServer.render`) was resolved in K-12: the
  HTML page writer is now `web.Server.writePage`, leaving `render` as
  a single-meaning name in code.

### set <!-- auto -->
CLI verb that creates or updates a key. Idempotent.
- `cmd/thimble/main.go:239-256` — `runSet`
- `cmd/thimble/main.go:500-508` — `(*store).SetSecret`
- ⚠ ambiguous: distinct from **create** (rejects existing) and
  **update** (rejects missing). Prose sometimes uses "set" loosely
  to mean any of the three.

### update (secret) <!-- auto -->
CLI verb that overwrites an existing key, failing if not present.
- `cmd/thimble/main.go:212-237` — `runWrite`
- `cmd/thimble/main.go:487-498` — `(*store).UpdateSecret`

### validate <!-- auto -->
Reject names/keys/recipients that don't match required patterns
before they reach the filesystem or `age`.
- `cmd/thimble/main.go:920-957` — `validateName` / `validateKey` /
  `validateRecipient(s)`

---

## Phrases

### "atomic write" <!-- auto -->
Write to a temp file in the same directory, fsync, rename. Used for
both manifest and bundle so a crashed writer never leaves a torn file.
- `cmd/thimble/main.go:711-733` — `atomicWrite`

### "encrypted bundle" <!-- auto -->
See **bundle** noun. Used in prose as the durable, peer-shippable
artifact. Always synonymous with **bundle** in this codebase.

### "masked prompt" <!-- auto -->
Terminal input where typed characters are not echoed; used by the CLI
for interactive secret entry.
- `cmd/thimble/main.go:844-867` — `secretInput`
- `README.md:74-80`

### "peer-to-peer sync" <!-- auto -->
Pattern of two or more peers exchanging encrypted bundles + manifest
through a shared transport (typically `git`) without a central server.
- `README.md:175-218`
- `thimble.md:96-118`

### "safe secret entry" <!-- auto -->
The README's umbrella for the no-argv-values rule and its tooling
(masked prompt, pipe input, `provision`, `and-set`, `and-get`).
- `README.md:71-115`

### "trust boundary" <!-- auto -->
Concrete after K-12. Each `internal/*` package now carries a doc
comment naming its trust boundary:
- `internal/age` — only outside-of-cmd plaintext-handling package; only
  package that exec()s the `age` binary.
- `internal/store` — only writer of the secrets directory.
- `internal/web` — only network-listener.
- `internal/cli` — only place where untrusted argv lands; validates
  before forwarding to store/web.
- `internal/dotenv` — pure parser/encoder; never touches FS or shell.

---

## Acronyms & Shorthand

| Short  | Expansion                                    | Notes                                                                     |
|--------|----------------------------------------------|---------------------------------------------------------------------------|
| CLI    | Command-Line Interface                       | The `thimble` binary.                                                     |
| TTY    | Teletypewriter (terminal)                    | Used in `provision`/`secretInput` checks.                                 |
| TOCTOU | Time-of-check, Time-of-use                   | Race condition class addressed by [K-21](tasks/knots/K-21-manifest-version-flock.md). |
| CSRF   | Cross-Site Request Forgery                   | Web UI risk; addressed in part by K-30/K-31.                              |
| DNS    | Domain Name System                           | DNS rebinding defense in [K-31](tasks/knots/K-31-host-header-allowlist.md). |
| GHCR   | GitHub Container Registry                    | Target of Thimble's published Docker image ([K-45](tasks/knots/K-45-distribution-docker.md)). |
| PVR    | (GitHub) Private Vulnerability Reporting     | Channel proposed in [K-06](tasks/knots/K-06-real-security-md.md).         |
| SLSA   | Supply-chain Levels for Software Artifacts   | Goal of release attestation ([K-40](tasks/knots/K-40-release-attestation.md)). |
| SOPS   | Secrets OPerationS (Mozilla)                 | Referenced as a heavier alternative in `thimble.md`.                      |

---

## Review Queue

Terms needing human attention. Resolve and remove.

- Resolved in K-12: **store** (Go type now `store.Store`, package
  qualifier disambiguates from `--store` flag and English verb).
- Resolved in K-12: **render** (HTML page writer is now
  `web.Server.writePage`; `render` is single-meaning in code).
- Resolved in K-12: **trust boundary** (each `internal/*` package
  carries a doc comment naming its trust boundary).
- ⚠ overloaded: **key** (secret name vs private key material).
- ⚠ overloaded: **environment** vs short form `env` (which is also `--env`).
- ⚠ ambiguous: **set** (CLI verb that absorbs create+update; some prose
  also uses "set" loosely for any write).
- ⚠ overloaded: **peer** vs **deploy host** in deployment-flow prose.
- ⚠ stale: **audit log** is in SECURITY.md residual risks but not
  implemented; lands with K-27.
- ⚠ divergence: thimble.md early sections use `thimble.toml` and a
  flat per-environment layout; the implementation uses
  `thimble.json` and `application/environment` namespaces. README is
  authoritative.
