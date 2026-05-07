# K-24 — `and-get --env` shell/docker guard

- Wave / Step: 4.7
- Effort: S
- Risk: med
- Deps: K-12
- Files: internal/cli/, README.md

## Goal

`and-get --env` exposes the secret through the child's environment block.
SECURITY.md flags it. We can do better: refuse to use `--env` for shell-shaped
children that almost always re-export.

## Acceptance

- When `--env` is set and the child command's basename is one of `sh`,
  `bash`, `zsh`, `fish`, `pwsh`, `cmd.exe`, `powershell`, abort with: "use
  stdin or `--allow-shell-env`; child shell will export the value."
- Same for `docker run` / `podman run` without `--env-file=-` or `-e KEY`
  pointing only at this var.
- `--allow-shell-env` flag exists for users who really mean it.
- README "Safe Secret Entry" section gains a paragraph explaining when
  `--env` is and isn't safe.

## Notes

This is a soft guard: a determined user can always work around it. Goal is
to catch the muscle-memory `thimble and-get --env FOO bar prod KEY -- bash
-c '...'` pattern that is almost always wrong.
