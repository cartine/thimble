# Lessons

Patterns from corrections. Review at session start; add to after any
correction.

## Git: rebase BEFORE commit + attribution note, not after

**What happened (2026-07-06):** committed to local main, attached the
sneka-agents git note to HEAD, then discovered origin/main had advanced.
`git pull --rebase` rewrote the commit SHA, orphaning the note; it had to
be re-attached with `-f` and the notes ref pushed again. This recurs.

**Rule:** the order is always

1. `git pull --rebase origin main` — sync FIRST, so the SHA is final
2. `git commit`
3. attach the sneka-agents note to HEAD
4. `git push origin main refs/notes/sneka-agents`

If the SHA changes after the note anyway (late rebase, `--amend`), the
note does not follow: verify with `git notes --ref=sneka-agents show HEAD`
and re-attach with `-f` before pushing the notes ref.

(The user-level sneka-agents skill now encodes this too — step 0 and the
Rules section.)
