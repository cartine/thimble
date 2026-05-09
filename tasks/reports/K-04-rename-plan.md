# K-04 Code-to-Taxonomy Rename Plan

Date: 2026-05-07
Knot: thimble-2bb2 ("Run /taxonomize-align-code")
Branch: claude/elastic-clarke-9c0ed3
Baseline: `go build ./...` ✓, `go test ./...` ✓

## Canonical-index audit

A pass over `cmd/thimble/main.go` and `cmd/thimble/main_test.go` against
[TAXONOMY.md](../../TAXONOMY.md) found that the bulk of code identifiers
already match the canonical taxonomy:

| Identifier (code)       | Taxonomy term     | Status |
|-------------------------|-------------------|--------|
| `manifest`              | manifest          | ✓ aligned |
| `appManifest`           | application + manifest | ✓ aligned |
| `envManifest`           | environment + manifest | ✓ aligned |
| `Recipients`            | recipient         | ✓ aligned |
| `secretEntry`           | secret            | ✓ aligned |
| `namespaceView`         | namespace         | ✓ aligned |
| `webServer`             | web UI            | ✓ aligned |
| `validateName/Key/Recipient(s)` | validate verb | ✓ aligned |
| `runInit/Set/Provision/AndSet/AndGet/Render/...` | CLI verbs | ✓ aligned |
| `(*store).encryptAndWrite/decrypt` | encrypt/decrypt verbs | ✓ aligned |
| `parseDotenv/encodeDotenv` | dotenv noun     | ✓ aligned |
| `atomicWrite`           | "atomic write" phrase | ✓ aligned |
| `redact`                | redact verb       | ✓ aligned |

## Renames recommended by K-02 drift report

These are the items the K-02 drift scan flagged as overloaded in code.
Both are `⚠ overloaded` in TAXONOMY.md, which the alignment skill
excludes from auto-rename.

### 1. Go type `store` (cmd/thimble/main.go:61)

- Current: `type store struct { root, agePath, identity string; now func() time.Time }`
- Conflict: collides semantically with the `--store` flag (a directory
  path) and the English verb "to store".
- Suggested rename: `secretStore` (parallels `secretEntry` already in
  the same file).
- Classification: **deferred** — better resolved by [K-12](../knots/K-12-split-main-go.md).
- Reason: K-12 moves this into a new package `internal/store/`, where
  Go-idiomatic naming becomes `store.Store` (or just `store.New()`
  returning the unexported `*store`). The package boundary makes the
  rename redundant.

### 2. Method `webServer.render` (cmd/thimble/main.go:1165)

- Current: `func (s *webServer) render(w http.ResponseWriter, r *http.Request, data pageData)`
- Conflict: collides with `(*store).Render` (decrypt + emit dotenv) and
  with `runRender`. Three different "render" verbs in the same file.
- Suggested rename: `writePage` (emphasizes the side-effect of writing
  HTML to a ResponseWriter, distinct from the dotenv-render path).
- Classification: **deferred** — better resolved by [K-12](../knots/K-12-split-main-go.md).
- Reason: K-12 moves this into `internal/web/`. The method becomes
  `web.Server.render`, which is no longer ambiguous because the type's
  package qualifies it. K-12 may still pick `writePage` for clarity;
  the decision moves there with full context.

## Auto-applied renames

```
paths:        0
packages:     0
types:        0
functions:    0
methods:      0
variables:    0
constants:    0
doc lines:    0
```

Nothing applied. The two candidate renames are deferred to K-12 per
the rationale above.

## Idempotency

A re-run of `/taxonomize-align-code` against an unchanged TAXONOMY.md
and source tree will reach the same conclusion: zero auto-rename
candidates remaining. The `⚠ overloaded` flags on `store` and `render`
in TAXONOMY.md are the gating mechanism.

## Verification

- `go build ./...` → exit 0 (unchanged from baseline).
- `go test ./...` → exit 0 (unchanged from baseline).
- `git diff` against the K-03 commit → empty (this knot is plan-only).
- Public CLI surface unchanged (no flags, subcommands, or arg names
  modified).
