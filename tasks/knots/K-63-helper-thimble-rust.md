# K-63 — Ship thimble-rust helper library

- Status: planned. Stays READY_FOR_PLANNING until Thimble v0.1.0+ ships.
- Knot id: see `kno ls -g ref:k-63`.
- Follow-up to: K-58 (`thimble exec`), K-59 (doc realignment).

## Goal

A tiny official Rust client library that wraps the `thimble exec` stdin
protocol with a single function:
`let secrets = thimble::read_secrets()?`. The library does NOT call
`age` or speak to the store directly — it parses the dotenv body
`thimble exec` already pipes into the child process.

## Why a library at all

See [K-60](K-60-helper-thimble-py.md) — same rationale, different
language. Idiomatic Rust callers expect `?`-propagation and a
`Result<HashMap<String, String>, Error>` return.

## Acceptance

1. Repository at `github.com/cartine/thimble-rust` with LICENSE
   (Apache-2.0), README, tests, CI.
2. Single public API:
   `pub fn read_secrets() -> std::io::Result<HashMap<String, String>>`.
   No other config knobs in v0.
3. Distribution: `cargo add thimble` (crate name `thimble`) works.
4. Parser handles the same dotenv subset as
   [`internal/dotenv/dotenv.go`](../../internal/dotenv/dotenv.go).
5. Tests cover: happy path, malformed line, empty stdin, large value
   (>64 KiB), trailing newlines.
6. README example shows the canonical pattern; links back to Thimble's
   main README "Consuming Secrets" section.
7. `docs.rs` renders cleanly with a runnable example.

## Constraints

- ZERO third-party deps. `std` only.
- API surface MUST stay tiny — one function.
- No mention of third-party vault products in the docs.
- v0.1.0 release waits until Thimble itself is at v0.1.0+.
