package cli

import (
	"errors"
	"path/filepath"
	"strings"
)

// shellChildren is the set of basenames whose mere presence as the
// child of `and-get --env` is almost certainly a foot-gun: the child
// will export the value to its descendants. Members come from the
// K-24 spec.
var shellChildren = map[string]bool{
	"sh":         true,
	"bash":       true,
	"zsh":        true,
	"fish":       true,
	"pwsh":       true,
	"cmd.exe":    true,
	"powershell": true,
}

// guardShellEnv refuses to use `and-get --env` when the child is a
// shell or a `docker run` / `podman run` invocation that would
// inherit the secret in its environment block. It returns nil when
// the child is safe or when the user has opted out via the calling
// site.
func guardShellEnv(cmdArgs []string, envVar string) error {
	if len(cmdArgs) == 0 {
		return nil
	}
	base := strings.ToLower(filepath.Base(cmdArgs[0]))
	if shellChildren[base] {
		return errors.New(
			"use stdin or --allow-shell-env; child shell will export the value",
		)
	}
	if base == "docker" || base == "podman" {
		if !dockerRunIsScoped(cmdArgs[1:], envVar) {
			return errors.New(
				"use stdin or --allow-shell-env; child shell will export the value",
			)
		}
	}
	return nil
}

// dockerRunIsScoped reports whether a `docker run` / `podman run`
// argument list narrowly scopes this var to the container: it must
// pass exactly `-e KEY` / `--env=KEY` referencing envVar, or read
// env from stdin via `--env-file=-`. Anything else (including no env
// flags at all) is treated as suspect because the operator's whole
// shell environment may be inherited by the container.
func dockerRunIsScoped(args []string, envVar string) bool {
	if len(args) == 0 || args[0] != "run" {
		// Not `docker run`; defer to caller's intent.
		return true
	}
	scoped := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-e", a == "--env":
			i++
			if i >= len(args) || !envFlagReferences(args[i], envVar) {
				return false
			}
			scoped = true
		case strings.HasPrefix(a, "--env="):
			if !envFlagReferences(strings.TrimPrefix(a, "--env="), envVar) {
				return false
			}
			scoped = true
		case a == "--env-file=-":
			scoped = true
		case strings.HasPrefix(a, "--env-file"):
			return false
		}
	}
	return scoped
}

// envFlagReferences returns true when a `-e KEY` value mentions
// exactly the var we care about (KEY or KEY=...).
func envFlagReferences(value, envVar string) bool {
	if value == envVar {
		return true
	}
	return strings.HasPrefix(value, envVar+"=")
}
