# K-03 Doc Alignment — Conflicts Report

Date: 2026-05-07
Knot: thimble-9ad5 ("Run /taxonomize-align-docs")

## Auto-applied changes

A scan of [README.md](../../README.md), [SECURITY.md](../../SECURITY.md), and
[thimble.md](../../thimble.md) against [TAXONOMY.md](../../TAXONOMY.md)
produced very little eligible drift to auto-fix. The docs were already
authored against the implementation's vocabulary; the candidate "aliases"
that did surface were carrying meaning rather than being synonym drift:

- `public recipient` (README.md ×4, SECURITY.md ×1) — kept; the "public"
  modifier is doing work in sentences that also mention "private identity."
- `encrypted dotenv bundle` (README.md ×1) — kept; descriptive opener, not
  drift.
- `multi-user` (SECURITY.md ×1) — correct English, not a vocabulary issue.
- `product user API keys` (thimble.md ×1) — refers to end-user keys (a
  different domain), not Thimble operators.

The single applied change:

- **thimble.md** — prepended a "status: original design doc, retained for
  context" callout that names the two specific points of divergence
  (`thimble.json` vs `thimble.toml`; namespaced vs flat layout) and points
  at the README as authoritative.

## Conflicts left for human resolution

These are contradictions that K-03 deliberately did NOT auto-rewrite. Per
the K-03 acceptance ("no new content added — only rewording for term
alignment") and the alignment skill's rule against silently fixing
contradictions, they're surfaced here.

### 1. thimble.md manifest format example (TOML vs JSON)

- thimble.md:60-76 shows an `[environment.production]` TOML block.
- The implementation only reads `thimble.json`
  (`cmd/thimble/main.go:33` — `manifestName = "thimble.json"`).

Recommended resolution: either (a) rewrite the example block to JSON, or
(b) leave it as a historical artifact alongside the new status header.
**Status header (b)** is what K-03 chose; (a) is a follow-up if the design
doc is to be kept current.

### 2. thimble.md flat layout example

- thimble.md:54-58 shows `secrets/production.env.age` directly under
  `secrets/`.
- Implementation is `secrets/<application>/<environment>.env.age`
  (`cmd/thimble/main.go:432-437`).

Same recommendation as #1.

### 3. thimble.md CLI sketches use a 1-positional model

- Lines 80-93, 126, 128, 139, 141, 144 show e.g.
  `thimble init production`, `thimble set production POSTGRES_PASSWORD`,
  `thimble render production --format dotenv`, with one positional arg.
- Current CLI requires `<application> <environment>` (two positionals).

Same recommendation as #1.

### 4. thimble.md verbs that don't exist yet

- `thimble edit production` (line 87)
- `thimble peer invite production alice-laptop` (line 111)
- `thimble peer sync production deploy@koja.dev` (line 112)
- `thimble peer pull production alice-laptop` (line 113)
- `thimble deploy production koja` (line 91, 128)
- `thimble rotate production KOJA_SMTP_PASSWORD` (line 92)

These are aspirational design-time examples. The status header signals
this; recommended to leave as is until the design doc is rewritten or
retired.

### 5. Operator vs operator laptop

- README uses both "operator" (the human) and "operator laptop" (the
  machine), sometimes within a few lines (e.g. `README.md:138` and
  `README.md:172`).
- TAXONOMY.md formalized "operator" as the human, "operator laptop" only
  when stressing the machine.
- Not auto-rewritten because every README occurrence already fits this
  rule.

### 6. Peer vs deploy host

- README uses "peer" as the umbrella, "deploy host" as a runtime peer
  (already consistent with TAXONOMY.md).
- thimble.md uses both more loosely; covered by the status header.

### 7. Set ambiguity

- README's "Safe Secret Entry" section uses "set" loosely
  (`README.md:71-115`).
- TAXONOMY.md flags `set` as ambiguous against `create`/`update`.
- Not auto-rewritten — replacing "set" with "set or update" reads
  awkwardly and doesn't change reader understanding meaningfully.

### 8. Store overload

- "Store" appears as: a directory (`--store` flag), a Go type, and a verb.
- TAXONOMY.md flags this as overloaded.
- Code-side disambiguation is K-04's job; docs use the verb sense
  predominantly, which is fine in English.

### 9. Stale taxonomy entries

- `audit log` is in SECURITY.md residual risks; lands with [K-27].
- `trust boundary` is in K-12 description; lands when packages have doc
  comments.

## Verification

- `grep "thimble.toml" README.md SECURITY.md` → 0 matches.
- `grep -E "\b(envfile|vault)\b" README.md SECURITY.md` (excluding
  references to actual products SOPS/Vault as alternatives) → 0
  problematic matches.
- A second `/taxonomize-align-docs` run with no source changes will find
  the same conflicts list (idempotent; nothing to apply).
