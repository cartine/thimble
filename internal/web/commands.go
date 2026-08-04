package web

import "strings"

func retrievalCommand(
	executable string, active StoreInfo, identity, app, env, key string,
) string {
	storeArg := active.Path
	if storeArg == "" {
		storeArg = active.Name
	}
	args := []string{shellQuote(executable), "--store", shellQuote(storeArg)}
	if identity != "" {
		args = append(args, "--identity", shellQuote(identity))
	}
	args = append(args, "get", app, env, key)
	return strings.Join(args, " ")
}

func shellQuote(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && !strings.ContainsRune("/._-", r)
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
