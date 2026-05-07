# K-03 — Run /taxonomize-align-docs

- Wave / Step: 0.3
- Effort: S
- Risk: low
- Deps: K-02
- Files: README.md, SECURITY.md, thimble.md

## Goal

Apply TAXONOMY.md to existing markdown. Replace synonyms with canonical terms,
flag contradictions, and produce a unified voice across all three docs.

## Acceptance

- README.md, SECURITY.md, and thimble.md all use canonical terminology from
  TAXONOMY.md exclusively.
- The skill's diff is reviewed and committed; no new content added — only
  rewording for term alignment.
- Any contradictions the skill could not auto-resolve are listed in
  `tasks/reports/K-03-doc-align-conflicts.md` for human resolution.
- A grep for legacy synonyms (e.g. "store", "vault", "envfile") returns no
  hits except inside fenced code blocks where removal would break the example.

## Notes

The README's "Real-Life Flow" deliberately uses concrete sample names
(`web-api`, `production`); leave those alone. Only realign nouns/verbs that are
part of Thimble's vocabulary, not user-supplied identifiers.
