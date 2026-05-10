# K-62 — Ship thimble-go helper library

- Status: planned. Stays READY_FOR_PLANNING until Thimble v0.1.0+ ships.
- Knot id: see `kno ls -g ref:k-62`.
- Follow-up to: K-58 (`thimble exec`), K-59 (doc realignment).

## Goal

A tiny official Go client library that wraps the `thimble exec` stdin
protocol with a single function: `secrets, err := thimble.ReadSecrets()`.
The library does NOT call `age` or speak to the store directly — it
parses the dotenv body `thimble exec` already pipes into the child
process.

Bonus: this library can simply re-export `internal/dotenv/dotenv.go` 's
parser as the public surface, since it's already a tested implementation.
That gives the Go ecosystem an extra-trustworthy option (the same parser
Thimble uses on the other side of the pipe).

## Acceptance

1. Repository at `github.com/cartine/thimble-go` with LICENSE
   (Apache-2.0), README, tests, CI.
2. Single public API: `func ReadSecrets() (map[string]string, error)`.
   Reads from `os.Stdin`. No other config knobs in v0.
3. Distribution: `go get github.com/cartine/thimble-go` works.
4. Parser is bit-for-bit identical to `internal/dotenv/dotenv.go` (or
   re-exports it via a small shim package).
5. Tests cover: happy path, malformed line, empty stdin, large value
   (>64 KiB), trailing newlines.
6. README example shows the canonical pattern; links back to Thimble's
   main README "Consuming Secrets" section.
7. `pkg.go.dev` shows clean docs: package-level doc comment, function
   doc comment, runnable example.

## Constraints

- ZERO third-party deps.
- API surface MUST stay tiny — one function.
- No mention of third-party vault products in the docs.
- v0.1.0 release waits until Thimble itself is at v0.1.0+.
