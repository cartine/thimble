# K-02 Taxonomy Drift Report

Date: 2026-05-07
Knot: thimble-ddde ("Run /taxonomize")

Sources scanned: README.md, SECURITY.md, thimble.md, CLAUDE.md, AGENTS.md,
cmd/thimble/main.go, cmd/thimble/main_test.go, all `tasks/knots/K-NN-*.md`.

## Summary

```
TAXONOMY.md updated
  + added:    47 terms
  ~ updated:  0 terms (auto-generated only)
  = untouched: 0 human-authored terms preserved
  ⚠ flagged:  9 terms (divergence/stale/overloaded/ambiguous)
```

## Drift items (cross-source inconsistencies)

These are the entries the next agent should resolve (in K-03 for docs,
K-04 for code).

### 1. `thimble.json` (impl) vs `thimble.toml` (design doc)

- README, code, and tests all use `thimble.json`
  (`cmd/thimble/main.go:33`, `manifestName = "thimble.json"`).
- thimble.md (the design doc) refers to `thimble.toml`
  (`thimble.md:55-76`).

Resolution: README is authoritative. K-03 should rewrite the
`thimble.toml` examples to JSON (or label them historical).

### 2. Manifest layout: flat per-environment vs application/environment

- thimble.md sketches a flat layout: `secrets/production.env.age`,
  `secrets/staging.env.age` (`thimble.md:55-58`).
- The implementation uses `secrets/<application>/<environment>.env.age`
  with an outer namespace key (`README.md:55-69`,
  `cmd/thimble/main.go:432-437`).

Resolution: keep the implementation; rewrite thimble.md examples.
This is a divergence flagged in TAXONOMY.md under **manifest** and
**namespace**.

### 3. `peer` vs `deploy host`

- README uses both terms with similar scope (`README.md:175-218`,
  `README.md:135-170`).
- thimble.md uses both, sometimes interchangeably
  (`thimble.md:97-118`).
- Code never names this concept (it's an external user of bundles).

Resolution: TAXONOMY.md treats **peer** as the umbrella, **deploy
host** as a specialization (a peer that decrypts at runtime). Docs
should use that distinction consistently.

### 4. `operator` overloaded with machine and human

- "Operator" sometimes means the human (`README.md:138`).
- Sometimes means the laptop (`README.md:172`,
  `README.md:215`).

Resolution: prefer **operator** for the human and **operator
laptop** when stressing the machine. Mostly a prose cleanup.

### 5. CLI verb `set` ambiguity

- `thimble set` is idempotent (create-or-update).
- `thimble create` rejects existing.
- `thimble update` rejects missing.
- README prose at `README.md:75-90` uses "set" loosely to cover all
  three.

Resolution: keep the three subcommands. Prose should say "set or
create" / "set or update" when ambiguity matters.

### 6. `key` overloading

- `key` = secret name (`DATABASE_URL`).
- `key` = age private key (informally in some prose).
- HTML template uses `key` for the form field.

Resolution: prefer **identity** for the age private key. Already
consistent in code; only minor prose drift.

### 7. `store` triple-meaning

- `--store` flag → directory path (`cmd/thimble/main.go:87`).
- `store` Go type → in-memory wrapper (`cmd/thimble/main.go:61`).
- "store" verb in prose → as in "store a secret".

Resolution: K-04 (taxonomize-align-code) should consider renaming the
Go type to `bundleStore` or `secretStore` so it doesn't shadow the
flag's meaning. Prose can keep "to store" as English.

### 8. `render` overloaded

- `thimble render` → decrypt + emit dotenv.
- `webServer.render` → write HTML page to ResponseWriter.

Resolution: K-04 should rename the web method to `writePage` or
`renderPage` to disambiguate. Trivial change.

### 9. Stale: `audit log` and `trust boundary`

- "Audit log" is named in SECURITY.md residual risks, in K-27, and in
  the security analysis, but no implementation exists yet.
- "Trust boundary" appears in K-12 description and this taxonomy but
  is not yet a concept in source comments.

Resolution: both lands with their owning knots (K-27 for audit log;
K-12 for explicit trust-boundary comments per package).

## Methodology

- Identifier extraction: walked types, methods, constants, regexes,
  and CLI subcommand strings in `cmd/thimble/main.go`.
- Prose harvest: read README, SECURITY, thimble.md, and the 48 knot
  drafts under `tasks/knots/`.
- Filter: dropped generic Go vocabulary (`context`, `error`, `string`,
  `map`, `slice`, `func`, `package`, `cmd`); kept domain-specific terms.
- Cluster: split into Nouns/Verbs/Phrases/Acronyms; flagged the 9
  drift items above.
- Idempotency: TAXONOMY.md was newly written; every entry marked
  `<!-- auto -->`. A second `/taxonomize` run on unchanged sources
  should produce a byte-identical file.
