# K-60 — Ship thimble-py helper library

- Status: planned. Stays READY_FOR_PLANNING until Thimble v0.1.0+ ships.
- Knot id: see `kno ls -g ref:k-60`.
- Follow-up to: K-58 (`thimble exec`), K-59 (doc realignment).

## Goal

A tiny official Python client library that wraps the `thimble exec`
stdin protocol with a single function: `secrets = thimble.read_secrets()`.
The library does NOT call `age` or speak to the store directly — it
parses the dotenv body `thimble exec` already pipes into the child
process.

## Why a library at all

The README's hand-rolled parser is ~25 lines and works fine. But:

1. Subtle parsing bugs in user code (escape handling, trailing newlines)
   are silent and dangerous.
2. New users have to read the README before they can integrate.
3. A one-liner is easier to evangelize: "your app reads stdin, calls one
   function, gets a dict."

The library is small enough to audit in 5 minutes, but standardizes the
protocol across languages.

## Acceptance

1. Repository at `github.com/cartine/thimble-py` with LICENSE
   (Apache-2.0), README, tests, CI.
2. Single public API: `thimble.read_secrets() -> dict[str, str]`. No
   other config knobs in v0.
3. Distribution: `pip install thimble` works.
4. Parser handles the same dotenv subset as
   [`internal/dotenv/dotenv.go`](../../internal/dotenv/dotenv.go):
   empty lines, `#` comments, unquoted simple values, quoted values
   with `\n` / `\r` / `\t` / `\\` / `\"` escapes.
5. Tests cover: happy path, malformed line, empty stdin, large value
   (>64 KiB), trailing newlines.
6. README example shows the canonical pattern; links back to Thimble's
   main README "Consuming Secrets" section.
7. Thimble's main README "Helper libraries" note (added in K-59) links
   to the published package once it ships.

## Constraints

- ZERO third-party deps. stdlib only.
- API surface MUST stay tiny — one function. Resist scope creep
  ("load from file", "merge with env vars", "auto-redact in logs").
  Each addition defeats the security model the protocol enforces.
- No mention of third-party vault products in the docs.
- v0.1.0 release waits until Thimble itself is at v0.1.0+.
